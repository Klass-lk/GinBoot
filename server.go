package ginboot

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"log/slog"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/config"
	"github.com/klass-lk/ginboot/service"
)

var osExit = os.Exit

type Runner func(engine *gin.Engine) error

type Server struct {
	engine        *gin.Engine
	runner        Runner
	corsConfig    *cors.Config
	basePath      string
	fileService   FileService
	logger        Logger
	scheduler     *Scheduler
	config        *config.Config
	serviceClient service.ServiceClient

	instrumentedMu    sync.Mutex
	instrumented      bool
	telemetryShutdown func(context.Context) error
}

func init() {
	// Customize route debug printing for Ginboot framework
	gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
		if gin.Mode() == gin.DebugMode {
			fmt.Printf("[GINBOOT-debug] %-6s %-45s (%d handlers)\n", httpMethod, absolutePath, nuHandlers)
		}
	}
}

func New() *Server {
	engine := gin.New()
	engine.Use(gin.Recovery())

	logger := NewSlogLogger(slog.Default())
	cfg, _ := config.LoadConfig("")
	svcClient := service.NewServiceClient(service.NewConfigServiceResolver(cfg))

	// Ensure .air.toml auto-reload configuration is present for development
	if gin.Mode() == gin.DebugMode {
		_ = EnsureAirConfig("")
	}

	srv := &Server{
		engine:        engine,
		logger:        logger,
		scheduler:     NewScheduler(logger),
		config:        cfg,
		serviceClient: svcClient,
	}

	// Before the caller registers a single route, because gin only applies
	// middleware to what is declared after it.
	srv.instrumentFromConfig()

	return srv
}

func (s *Server) Config() *config.Config {
	return s.config
}

func (s *Server) SetConfig(cfg *config.Config) *Server {
	s.config = cfg
	if s.serviceClient != nil {
		s.serviceClient.SetResolver(service.NewConfigServiceResolver(cfg))
	}
	return s
}

func (s *Server) ServiceClient() service.ServiceClient {
	return s.serviceClient
}

func (s *Server) SetServiceClient(client service.ServiceClient) *Server {
	s.serviceClient = client
	return s
}

func (s *Server) Scheduler() *Scheduler {
	return s.scheduler
}

// RegisterWorker registers a scheduled background worker with a custom time interval.
func (s *Server) RegisterWorker(name string, interval time.Duration, fn TaskFunc) {
	s.scheduler.RegisterTask(TaskOptions{
		Name:         name,
		Interval:     interval,
		RunOnStartup: true,
	}, fn)
}

// RegisterWorkerStruct registers a struct implementing the Worker interface.
func (s *Server) RegisterWorkerStruct(worker Worker) {
	s.RegisterWorker(worker.Name(), worker.Interval(), worker.Execute)
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}

// IsExportingSwagger returns true if the current execution is for Swagger/OpenAPI spec generation.
func IsExportingSwagger() bool {
	return os.Getenv("GINBOOT_EXPORT_SWAGGER") != ""
}

func (s *Server) registerDefaultHealthRoutes() {
	healthHandler := func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "UP",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"service":   "ginboot",
		})
	}

	s.engine.GET("/healthz", healthHandler)
	s.engine.GET("/health", healthHandler)

	if s.basePath != "" && s.basePath != "/" {
		p := s.basePath
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		s.engine.GET(path.Join(p, "healthz"), healthHandler)
		s.engine.GET(path.Join(p, "health"), healthHandler)
	}
}

func (s *Server) Start(port int) error {
	s.registerDefaultHealthRoutes()

	// After the application's routes, because the specification it serves is
	// built as those routes are registered.
	s.registerOpenAPIEndpoint()

	exportPath := os.Getenv("GINBOOT_EXPORT_SWAGGER")
	if exportPath != "" {
		err := exportOpenAPISpec(exportPath)
		if err != nil {
			fmt.Printf("Failed to export OpenAPI spec: %v\n", err)
			osExit(1)
			return err
		}
		fmt.Printf("Successfully exported OpenAPI spec to %s\n", exportPath)
		osExit(0)
		return nil
	}

	if s.runner != nil {
		return s.runner(s.engine)
	}

	if s.scheduler != nil && len(s.scheduler.GetTasks()) > 0 {
		ctx := context.Background()
		s.scheduler.Start(ctx)
	}

	return s.startHTTP(port)
}

func (s *Server) startHTTP(port int) error {
	addr := fmt.Sprintf(":%d", port)
	return s.engine.Run(addr)
}

func (s *Server) SetRunner(runner Runner) {
	s.runner = runner
}

func (s *Server) SetLogger(logger Logger) *Server {
	s.logger = logger
	return s
}

func (s *Server) SetBasePath(path string) *Server {
	s.basePath = path
	return s
}

func (s *Server) WithCORS(config *cors.Config) *Server {
	s.corsConfig = config
	s.engine.Use(cors.New(*config))
	return s
}

func (s *Server) BindFileService(fileService FileService) *Server {
	s.fileService = fileService
	return s
}

func (s *Server) DefaultCORS() *Server {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-Request-ID", "x-request-id"}
	config.ExposeHeaders = []string{"X-Request-ID", "x-request-id"}
	config.MaxAge = 12 * time.Hour
	return s.WithCORS(&config)
}

func (s *Server) CustomCORS(allowOrigins []string, allowMethods []string, allowHeaders []string, maxAge time.Duration) *Server {
	config := cors.Config{
		AllowOrigins: allowOrigins,
		AllowMethods: allowMethods,
		AllowHeaders: allowHeaders,
		MaxAge:       maxAge,
	}
	return s.WithCORS(&config)
}

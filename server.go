package ginboot

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"log/slog"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

var osExit = os.Exit

type Runner func(engine *gin.Engine) error

type Server struct {
	engine      *gin.Engine
	runner      Runner
	corsConfig  *cors.Config
	basePath    string
	fileService FileService
	logger      Logger
	scheduler   *Scheduler
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
	return &Server{
		engine:    engine,
		logger:    logger,
		scheduler: NewScheduler(logger),
	}
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

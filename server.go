package ginboot

import (
	"fmt"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Runner func(engine *gin.Engine) error

type Server struct {
	engine      *gin.Engine
	runner      Runner
	corsConfig  *cors.Config
	basePath    string
	fileService FileService
}

func New() *Server {
	return &Server{
		engine: gin.Default(),
	}
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) Start(port int) error {
	if s.runner != nil {
		return s.runner(s.engine)
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
	config.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
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

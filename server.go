package ginboot

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Runtime string

const (
	RuntimeLambda Runtime = "lambda"
	RuntimeHTTP   Runtime = "http"
)

type Server struct {
	engine     *gin.Engine
	runtime    Runtime
	corsConfig *cors.Config
}

func New() *Server {
	runtime := RuntimeHTTP
	if os.Getenv("LAMBDA_RUNTIME") == "true" {
		runtime = RuntimeLambda
	}

	return &Server{
		engine:  gin.Default(),
		runtime: runtime,
	}
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) Start(port int) error {
	if s.runtime == RuntimeLambda {
		return s.startLambda()
	}
	return s.startHTTP(port)
}

func (s *Server) startHTTP(port int) error {
	addr := fmt.Sprintf(":%d", port)
	return s.engine.Run(addr)
}

func (s *Server) startLambda() error {
	ginLambda := ginadapter.New(s.engine)

	handler := func(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		return ginLambda.ProxyWithContext(ctx, req)
	}

	lambda.Start(handler)
	return nil
}

func (s *Server) SetRuntime(runtime Runtime) {
	s.runtime = runtime
}

func (s *Server) WithCORS(config *cors.Config) *Server {
	s.corsConfig = config
	s.engine.Use(cors.New(*config))
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

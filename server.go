package ginboot

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-gonic/gin"
)

type Runtime string

const (
	RuntimeLambda Runtime = "lambda"
	RuntimeHTTP   Runtime = "http"
)

type Server struct {
	engine  *gin.Engine
	runtime Runtime
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

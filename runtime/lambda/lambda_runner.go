package lambda

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"
)

func NewRunner() ginboot.Runner {
	return func(engine *gin.Engine) error {
		ginLambda := ginadapter.New(engine)

		handler := func(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
			return ginLambda.ProxyWithContext(ctx, req)
		}

		lambda.Start(handler)
		return nil
	}
}

// NewRunnerWithTelemetry creates a runner that wraps the lambda handler with OpenTelemetry.
func NewRunnerWithTelemetry() ginboot.Runner {
	return func(engine *gin.Engine) error {
		ginLambda := ginadapter.New(engine)

		handler := func(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
			return ginLambda.ProxyWithContext(ctx, req)
		}

		// Wrap with OpenTelemetry
		instrumentedHandler := otellambda.InstrumentHandler(handler,
			otellambda.WithFlusher(otellambda.NewFlusher()),
		)

		lambda.Start(instrumentedHandler)
		return nil
	}
}

// NewRunnerV2 creates a runner for AWS API Gateway HTTP APIs (Payload v2.0).
func NewRunnerV2() ginboot.Runner {
	return func(engine *gin.Engine) error {
		ginLambda := ginadapter.NewV2(engine)

		handler := func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
			return ginLambda.ProxyWithContext(ctx, req)
		}

		lambda.Start(handler)
		return nil
	}
}

// NewRunnerV2WithTelemetry creates a runner for HTTP APIs (Payload v2.0) with OpenTelemetry.
func NewRunnerV2WithTelemetry() ginboot.Runner {
	return func(engine *gin.Engine) error {
		ginLambda := ginadapter.NewV2(engine)

		handler := func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
			return ginLambda.ProxyWithContext(ctx, req)
		}

		instrumentedHandler := otellambda.InstrumentHandler(handler,
			otellambda.WithFlusher(otellambda.NewFlusher()),
		)

		lambda.Start(instrumentedHandler)
		return nil
	}
}

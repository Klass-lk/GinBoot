package lambda

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
)

func NewRunner() ginboot.Runner {
	return func(engine *gin.Engine) error {
		ginLambdaV1 := ginadapter.New(engine)
		ginLambdaV2 := ginadapter.NewV2(engine)

		handler := func(ctx context.Context, req json.RawMessage) (interface{}, error) {
			reqStr := string(req)
			if strings.Contains(reqStr, `"version":"2.0"`) || strings.Contains(reqStr, `"version": "2.0"`) {
				var v2Req events.APIGatewayV2HTTPRequest
				if err := json.Unmarshal(req, &v2Req); err == nil && v2Req.Version == "2.0" {
					return ginLambdaV2.ProxyWithContext(context.Background(), v2Req)
				}
			}

			var v1Req events.APIGatewayProxyRequest
			if err := json.Unmarshal(req, &v1Req); err == nil && (v1Req.HTTPMethod != "" || v1Req.Resource != "") {
				return ginLambdaV1.ProxyWithContext(context.Background(), v1Req)
			}

			var v2Req events.APIGatewayV2HTTPRequest
			if err := json.Unmarshal(req, &v2Req); err == nil && v2Req.RequestContext.HTTP.Method != "" {
				return ginLambdaV2.ProxyWithContext(context.Background(), v2Req)
			}

			return ginLambdaV1.ProxyWithContext(context.Background(), v1Req)
		}

		lambda.Start(handler)
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

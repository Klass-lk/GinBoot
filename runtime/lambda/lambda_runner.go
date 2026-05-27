package lambda

import (
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
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

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

// NewRunnerFor builds the runner for a server, wiring everything that server
// registered.
//
// This is the constructor to use. The two below take their collaborators as
// arguments, which means an application that registered workers and called
// NewRunner() gets a runtime that silently runs none of them — there is nothing
// to notice, because a job that never fires looks exactly like a job that is not
// due yet. Adding consumers to that scheme would have added a third way to be
// quietly wrong, so the server is passed whole and the runner reads what it
// needs.
func NewRunnerFor(server *ginboot.Server) ginboot.Runner {
	if server == nil {
		return newRunner(nil, nil)
	}
	return newRunner(server.Scheduler(), server.Consumers())
}

// NewRunner builds a runner with no scheduler and no consumers.
//
// Deprecated: use NewRunnerFor, which cannot leave registered workers or
// consumers unwired.
func NewRunner() ginboot.Runner {
	return newRunner(nil, nil)
}

// NewRunnerWithScheduler builds a runner that can run scheduled workers.
//
// Deprecated: use NewRunnerFor, which also wires event consumers.
func NewRunnerWithScheduler(scheduler *ginboot.Scheduler) ginboot.Runner {
	return newRunner(scheduler, nil)
}

func newRunner(scheduler *ginboot.Scheduler, consumers *ginboot.ConsumerRegistry) ginboot.Runner {
	return func(engine *gin.Engine) error {
		ginLambdaV1 := ginadapter.New(engine)
		ginLambdaV2 := ginadapter.NewV2(engine)

		// Buffered telemetry has to leave before the environment freezes, and the
		// only window for that is after the response. Nil unless there is
		// something to drain and Lambda accepted the registration; signalling it
		// is a no-op either way. See extension.go.
		extension := startTelemetryExtension()

		handler := func(ctx context.Context, req json.RawMessage) (interface{}, error) {
			// Deferred so it runs however the handler leaves: an invocation that
			// panicked or errored still produced telemetry, and is usually the
			// one worth having.
			defer extension.invocationComplete()

			// Check if incoming payload is a cloud scheduled event (AWS EventBridge, GCP, Azure, HTTP)
			if scheduler != nil {
				if event, ok := scheduler.ParseScheduledEvent(req); ok {
					if event.TaskName != "" {
						err := scheduler.ExecuteWorkerByName(ctx, event.TaskName)
						return map[string]interface{}{"status": "scheduled worker executed", "provider": event.Provider, "task": event.TaskName}, err
					}
					// An unnamed rule is the per-application tick: it says "look for
					// work", not "run everything". Running everything would give
					// each worker the tick's schedule instead of its own — an
					// hourly job firing twelve times an hour, a daily one two
					// hundred and eighty-eight times.
					results := scheduler.ExecuteDueWorkers(ctx)
					return map[string]interface{}{"status": "due workers executed", "provider": event.Provider, "results": results}, nil
				}
			}

			// Events, before the API Gateway branches below.
			//
			// Ordered after the scheduler's parser deliberately: the scheduler's
			// tick is itself an EventBridge rule, and an event matcher placed
			// first would swallow it. Ordered before the HTTP branches because
			// those end in a fallback that treats an unrecognised payload as a v1
			// proxy request — an SQS batch reaching that far would be answered
			// with a 404 page and acknowledged as handled.
			if consumers != nil && consumers.Len() > 0 {
				if event, ok := looksLikeSQS(req); ok {
					return handleSQS(ctx, consumers, event)
				}
			}

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

		extension := startTelemetryExtension()

		handler := func(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
			defer extension.invocationComplete()
			return ginLambda.ProxyWithContext(ctx, req)
		}

		lambda.Start(handler)
		return nil
	}
}

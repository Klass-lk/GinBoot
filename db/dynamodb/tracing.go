package dynamodb

import (
	"context"
	"errors"
	"reflect"

	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/klass-lk/ginboot/db/dynamodb"

// WithTracing is a dynamodb.Options function that records each call
// (retries included) as one OpenTelemetry client span, named
// "<Operation> <table>" (e.g. "GetItem users") and parented by the call's
// context. A span carries the operation and table; a failed call marks it
// as an error with the service's error code (e.g.
// ConditionalCheckFailedException), never the message. Keys, items and
// expressions are never recorded.
//
//	client := dynamodb.NewFromConfig(cfg, ginbootdynamo.WithTracing)
//
// NewDynamoDBClient installs it. Spans that are not recorded (telemetry
// off, sampled out) cost a context lookup and nothing more.
func WithTracing(o *dynamodb.Options) {
	o.APIOptions = append(o.APIOptions, tracingMiddleware(nil))
}

func tracingMiddleware(tracer trace.Tracer) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		t := tracer
		if t == nil {
			t = otel.GetTracerProvider().Tracer(tracerName)
		}
		// Initialize runs once per call, before retries.
		return stack.Initialize.Add(middleware.InitializeMiddlewareFunc("ginboot:tracing",
			func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (
				middleware.InitializeOutput, middleware.Metadata, error) {
				op := awsmiddleware.GetOperationName(ctx)
				table := tableName(in.Parameters)
				name := op
				if table != "" {
					name += " " + table
				}
				ctx, span := t.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient))
				if !span.IsRecording() {
					return next.HandleInitialize(ctx, in)
				}
				defer span.End()
				span.SetAttributes(
					attribute.String("db.system", "aws.dynamodb"),
					attribute.String("db.operation.name", op),
					attribute.String("db.collection.name", table),
				)
				out, md, err := next.HandleInitialize(ctx, in)
				if err != nil {
					span.SetStatus(codes.Error, failure(err))
				}
				return out, md, err
			}), middleware.After)
	}
}

// tableName is the input's TableName, "" for calls over several tables
// (BatchGetItem, TransactWriteItems...).
func tableName(params any) string {
	v := reflect.ValueOf(params)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	f := v.FieldByName("TableName")
	if !f.IsValid() || f.Kind() != reflect.Pointer || f.IsNil() || f.Elem().Kind() != reflect.String {
		return ""
	}
	return f.Elem().String()
}

// failure names an error by the service's code, not its message.
func failure(err error) string {
	var api smithy.APIError
	if errors.As(err, &api) {
		return api.ErrorCode()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err.Error()
	}
	return "request failed"
}

package dynamodb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWithTracingRecordsCallSpans(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if strings.HasSuffix(r.Header.Get("X-Amz-Target"), "PutItem") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"__type":"com.amazonaws.dynamodb.v20120810#ConditionalCheckFailedException","message":"item secret@example.com exists"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	client := dynamodb.New(dynamodb.Options{
		Region:           "ap-southeast-1",
		BaseEndpoint:     aws.String(server.URL),
		Credentials:      credentials.NewStaticCredentialsProvider("k", "s", ""),
		RetryMaxAttempts: 1,
	}, func(o *dynamodb.Options) { o.APIOptions = append(o.APIOptions, tracingMiddleware(tp.Tracer("test"))) })

	ctx, parent := tp.Tracer("test").Start(context.Background(), "request")
	key := map[string]types.AttributeValue{"id": &types.AttributeValueMemberS{Value: "secret@example.com"}}
	_, err := client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String("users"), Key: key})
	require.NoError(t, err)
	_, err = client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String("users"), Item: key})
	require.Error(t, err)
	_, err = client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{RequestItems: map[string]types.KeysAndAttributes{"users": {Keys: []map[string]types.AttributeValue{key}}}})
	require.NoError(t, err)
	parent.End()

	spans := recorder.Ended()
	require.Len(t, spans, 4)
	assert.Equal(t, "GetItem users", spans[0].Name())
	assert.Equal(t, parent.SpanContext().SpanID(), spans[0].Parent().SpanID())
	assert.Contains(t, spans[0].Attributes(), attribute.String("db.system", "aws.dynamodb"))
	assert.Equal(t, "PutItem users", spans[1].Name())
	assert.Equal(t, codes.Error, spans[1].Status().Code)
	assert.Equal(t, "ConditionalCheckFailedException", spans[1].Status().Description)
	assert.Equal(t, "BatchGetItem", spans[2].Name(), "no single table")
	for _, s := range spans {
		for _, a := range s.Attributes() {
			assert.NotContains(t, a.Value.Emit(), "secret")
		}
	}
}

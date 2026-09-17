package lambda

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/klass-lk/ginboot"
)

func sqsRecord(id, arn, body string) events.SQSMessage {
	return events.SQSMessage{
		MessageId:      id,
		EventSource:    "aws:sqs",
		EventSourceARN: arn,
		Body:           body,
	}
}

func registryWith(t *testing.T, consumers ...ginboot.Consumer) *ginboot.ConsumerRegistry {
	t.Helper()
	r := ginboot.NewConsumerRegistry(ginboot.NewSlogLogger(nil))
	for _, c := range consumers {
		if err := r.Register(c); err != nil {
			t.Fatalf("registering: %v", err)
		}
	}
	return r
}

const smsARN = "arn:aws:sqs:ap-southeast-1:123456789012:Ginboot-a-b-sms"

func TestHandleSQSAcknowledgesSuccessfulBatch(t *testing.T) {
	var seen []string
	registry := registryWith(t, ginboot.NewQueueConsumer("sms", ginboot.Queue("sms"),
		func(ctx context.Context, m map[string]any) error {
			seen = append(seen, m["id"].(string))
			return nil
		}))

	response, err := handleSQS(context.Background(), registry, events.SQSEvent{Records: []events.SQSMessage{
		sqsRecord("m1", smsARN, `{"id":"one"}`),
		sqsRecord("m2", smsARN, `{"id":"two"}`),
	}})
	if err != nil {
		t.Fatalf("handling a good batch returned an error: %v", err)
	}
	if len(response.BatchItemFailures) != 0 {
		t.Errorf("a fully successful batch reported failures: %v", response.BatchItemFailures)
	}
	if len(seen) != 2 {
		t.Errorf("the handler saw %v", seen)
	}
}

// A queue nothing consumes must not be acknowledged: acknowledging deletes
// messages nothing has read, and the queue goes quietly empty.
func TestHandleSQSFailsEveryRecordOfAnUnknownQueue(t *testing.T) {
	registry := registryWith(t, ginboot.NewQueueConsumer("sms", ginboot.Queue("sms"),
		func(ctx context.Context, m map[string]any) error { return nil }))

	other := "arn:aws:sqs:ap-southeast-1:123456789012:Ginboot-a-b-unclaimed"
	response, err := handleSQS(context.Background(), registry, events.SQSEvent{Records: []events.SQSMessage{
		sqsRecord("m1", other, `{}`),
		sqsRecord("m2", other, `{}`),
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(response.BatchItemFailures) != 2 {
		t.Fatalf("records for an unconsumed queue were acknowledged: %v", response.BatchItemFailures)
	}
}

// One malformed message in a batch of three must redeliver only itself.
func TestHandleSQSReportsOnlyTheBadRecord(t *testing.T) {
	type message struct {
		ID string `json:"id"`
	}
	registry := registryWith(t, ginboot.NewQueueConsumer("sms", ginboot.Queue("sms"),
		func(ctx context.Context, m message) error { return nil }))

	response, err := handleSQS(context.Background(), registry, events.SQSEvent{Records: []events.SQSMessage{
		sqsRecord("m1", smsARN, `{"id":"one"}`),
		sqsRecord("m2", smsARN, `this is not json`),
		sqsRecord("m3", smsARN, `{"id":"three"}`),
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(response.BatchItemFailures) != 1 || response.BatchItemFailures[0].ItemIdentifier != "m2" {
		t.Fatalf("expected only m2 to be redelivered, got %v", response.BatchItemFailures)
	}
}

// A topic delivering into a queue wraps the publisher's message. The handler's
// type is the thing the publisher sent, not the envelope it travelled in.
func TestHandleSQSUnwrapsSNSEnvelope(t *testing.T) {
	type message struct {
		Text string `json:"text"`
	}
	var got message
	registry := registryWith(t, ginboot.NewQueueConsumer("sms", ginboot.Queue("sms"),
		func(ctx context.Context, m message) error {
			got = m
			return nil
		}))

	envelope, _ := json.Marshal(map[string]string{
		"Type":     "Notification",
		"TopicArn": "arn:aws:sns:ap-southeast-1:123456789012:alerts",
		"Message":  `{"text":"published"}`,
	})

	if _, err := handleSQS(context.Background(), registry, events.SQSEvent{
		Records: []events.SQSMessage{sqsRecord("m1", smsARN, string(envelope))},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Text != "published" {
		t.Fatalf("the SNS envelope was not unwrapped: %+v", got)
	}
}

// An application's own message that happens to have a "Type" field must not be
// mistaken for an SNS envelope and emptied.
func TestUnwrapLeavesOrdinaryMessagesAlone(t *testing.T) {
	body := []byte(`{"Type":"invoice","amount":42}`)
	if got := string(unwrapSNSEnvelope(body)); got != string(body) {
		t.Fatalf("an ordinary message was unwrapped as an SNS envelope: %s", got)
	}
}

func TestLooksLikeSQSRejectsNonSQSPayloads(t *testing.T) {
	// An API Gateway request carrying a queue ARN in its body would satisfy a
	// naive substring check and be routed to a consumer.
	apiRequest := []byte(`{"httpMethod":"POST","path":"/queues","body":"arn:aws:sqs:ap-southeast-1:123456789012:x","resource":"/{proxy+}"}`)
	if _, ok := looksLikeSQS(apiRequest); ok {
		t.Fatal("an API Gateway request was detected as an SQS batch")
	}

	if _, ok := looksLikeSQS([]byte(`{"Records":[]}`)); ok {
		t.Error("an empty Records array was detected as an SQS batch")
	}

	s3Event := []byte(`{"Records":[{"eventSource":"aws:s3","s3":{"bucket":{"name":"b"}}}]}`)
	if _, ok := looksLikeSQS(s3Event); ok {
		t.Error("an S3 event was detected as an SQS batch")
	}
}

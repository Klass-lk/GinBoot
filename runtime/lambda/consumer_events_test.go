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

// Nothing to route. Both are reachable in production: a function whose last
// consumer was removed, and a warm invocation handed an empty batch.
func TestHandleSQSWithNothingToDo(t *testing.T) {
	registry := registryWith(t)

	response, err := handleSQS(context.Background(), registry, events.SQSEvent{})
	if err != nil || len(response.BatchItemFailures) != 0 {
		t.Fatalf("an empty batch returned %v / %v", response, err)
	}

	response, err = handleSQS(context.Background(), nil, events.SQSEvent{
		Records: []events.SQSMessage{sqsRecord("m1", smsARN, `{}`)},
	})
	if err != nil || len(response.BatchItemFailures) != 0 {
		t.Fatalf("a nil registry returned %v / %v", response, err)
	}
}

// The handler sees SQS's own attributes and the sender's own, merged, with the
// sender's winning — someone asking for "TraceId" wants the one they set.
func TestSQSAttributesMergeSystemAndMessageAttributes(t *testing.T) {
	traceID := "sender-trace"
	binary := events.SQSMessageAttribute{DataType: "Binary", BinaryValue: []byte{0x01}}

	got := sqsAttributes(events.SQSMessage{
		Attributes: map[string]string{
			"SentTimestamp": "1700000000000",
			"TraceId":       "sqs-trace",
		},
		MessageAttributes: map[string]events.SQSMessageAttribute{
			"TraceId": {DataType: "String", StringValue: &traceID},
			"Blob":    binary,
		},
	})

	if got["SentTimestamp"] != "1700000000000" {
		t.Errorf("a system attribute was lost: %v", got)
	}
	if got["TraceId"] != traceID {
		t.Errorf("the sender's TraceId did not win: %q", got["TraceId"])
	}
	// A binary attribute has no string form, and a map of strings that sometimes
	// holds base64 is worse than one that never does.
	if _, present := got["Blob"]; present {
		t.Errorf("a binary attribute was flattened into the string map: %v", got)
	}
}

// The handler that needs more than Event models gets the record as it arrived.
func TestRawRecordCarriesTheWholeMessage(t *testing.T) {
	raw := rawRecord(sqsRecord("m1", smsARN, `{"id":"one"}`))
	if len(raw) == 0 {
		t.Fatal("the raw record is empty")
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the raw record is not readable JSON: %v", err)
	}
	if decoded["messageId"] != "m1" {
		t.Errorf("the raw record lost its message id: %v", decoded)
	}
}

func TestLooksLikeSQSRejectsUndecodablePayloads(t *testing.T) {
	// A scheduled event, a bare string, and something that is not JSON at all.
	for _, payload := range []string{
		`{"source":"aws.events","detail-type":"Scheduled Event"}`,
		`"just a string"`,
		`not json at all`,
	} {
		if _, ok := looksLikeSQS([]byte(payload)); ok {
			t.Errorf("%s was detected as an SQS batch", payload)
		}
	}
}

// A batch whose records claim different queues is split, and each half goes to
// the consumer that declared it.
func TestHandleSQSSplitsAMixedBatchByQueue(t *testing.T) {
	const emailARN = "arn:aws:sqs:ap-southeast-1:123456789012:Ginboot-a-b-email"

	var smsSeen, emailSeen []string
	registry := registryWith(t,
		ginboot.NewQueueConsumer("sms", ginboot.Queue("sms"),
			func(ctx context.Context, m map[string]any) error {
				smsSeen = append(smsSeen, m["id"].(string))
				return nil
			}),
		ginboot.NewQueueConsumer("email", ginboot.Queue("email"),
			func(ctx context.Context, m map[string]any) error {
				emailSeen = append(emailSeen, m["id"].(string))
				return nil
			}),
	)

	if _, err := handleSQS(context.Background(), registry, events.SQSEvent{Records: []events.SQSMessage{
		sqsRecord("m1", smsARN, `{"id":"s1"}`),
		sqsRecord("m2", emailARN, `{"id":"e1"}`),
		sqsRecord("m3", smsARN, `{"id":"s2"}`),
	}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(smsSeen) != 2 || len(emailSeen) != 1 {
		t.Fatalf("the batch was split as sms=%v email=%v", smsSeen, emailSeen)
	}
	if emailSeen[0] != "e1" {
		t.Errorf("the email consumer received %v", emailSeen)
	}
}

// The detection path as it actually runs: a raw Lambda payload, not a struct
// somebody already parsed. Everything else here calls handleSQS directly, which
// steps over the one function that decides whether a payload is an SQS batch at
// all.
func TestLooksLikeSQSAcceptsARealPayload(t *testing.T) {
	payload := []byte(`{
	  "Records": [
	    {
	      "messageId": "059f36b4-87a3-44ab-83d2-661975830a7d",
	      "receiptHandle": "AQEBwJnKyrHigUMZj6rYigCgxlaS3SLy0a...",
	      "body": "{\"text\":\"hello\"}",
	      "attributes": {
	        "ApproximateReceiveCount": "1",
	        "SentTimestamp": "1545082649183"
	      },
	      "messageAttributes": {},
	      "md5OfBody": "e4e68fb7bd0e697a0ae8f1bb342846b3",
	      "eventSource": "aws:sqs",
	      "eventSourceARN": "arn:aws:sqs:ap-southeast-1:123456789012:Ginboot-a-b-sms",
	      "awsRegion": "ap-southeast-1"
	    }
	  ]
	}`)

	event, ok := looksLikeSQS(payload)
	if !ok {
		t.Fatal("a real SQS payload was not detected; it would have been answered with a 404 page and acknowledged as handled")
	}
	if len(event.Records) != 1 || event.Records[0].MessageId != "059f36b4-87a3-44ab-83d2-661975830a7d" {
		t.Fatalf("the payload parsed as %+v", event.Records)
	}

	// And it routes end to end from there.
	var got string
	registry := registryWith(t, ginboot.NewQueueConsumer("sms", ginboot.Queue("sms"),
		func(ctx context.Context, m struct {
			Text string `json:"text"`
		}) error {
			got = m.Text
			return nil
		}))

	response, err := handleSQS(context.Background(), registry, event)
	if err != nil || len(response.BatchItemFailures) != 0 {
		t.Fatalf("routing the detected batch returned %v / %v", response, err)
	}
	if got != "hello" {
		t.Errorf("the handler received %q", got)
	}
}

// A batch that mixes SQS with anything else is not an SQS batch. Treating it as
// one would hand a consumer a record it cannot read.
func TestLooksLikeSQSRejectsAMixedRecordSet(t *testing.T) {
	mixed := []byte(`{"Records":[
	  {"eventSource":"aws:sqs","eventSourceARN":"arn:aws:sqs:x:1:q","messageId":"m1","body":"{}"},
	  {"eventSource":"aws:s3","s3":{"bucket":{"name":"b"}}}
	]}`)
	if _, ok := looksLikeSQS(mixed); ok {
		t.Fatal("a mixed record set was detected as an SQS batch")
	}
}

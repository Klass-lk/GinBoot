package ginboot

import (
	"context"
	"errors"
	"testing"
)

type testConsumer struct {
	name    string
	source  EventSource
	handled []string
	fail    map[string]error
	panicOn string
}

func (c *testConsumer) Name() string        { return c.name }
func (c *testConsumer) Source() EventSource { return c.source }
func (c *testConsumer) Handle(ctx context.Context, ev Event) error {
	if ev.ID == c.panicOn {
		panic("handler exploded")
	}
	c.handled = append(c.handled, ev.ID)
	if err, ok := c.fail[ev.ID]; ok {
		return err
	}
	return nil
}

func queueConsumerFor(name, ref string, managed bool) *testConsumer {
	return &testConsumer{
		name:   name,
		source: EventSource{Kind: SourceQueue, Ref: ref, Managed: managed},
		fail:   map[string]error{},
	}
}

func newTestRegistry(t *testing.T, consumers ...Consumer) *ConsumerRegistry {
	t.Helper()
	r := NewConsumerRegistry(NewSlogLogger(nil))
	for _, c := range consumers {
		if err := r.Register(c); err != nil {
			t.Fatalf("registering %s: %v", c.Name(), err)
		}
	}
	return r
}

func TestRegisterRejectsDuplicateName(t *testing.T) {
	r := newTestRegistry(t, queueConsumerFor("sms", "sms", true))
	err := r.Register(queueConsumerFor("sms", "other", true))
	if err == nil {
		t.Fatal("a second consumer under the same name was accepted; the first one's messages would go to a handler nobody expects")
	}
}

func TestRegisterRejectsIncompleteConsumer(t *testing.T) {
	r := NewConsumerRegistry(NewSlogLogger(nil))
	if err := r.Register(&testConsumer{name: "", source: EventSource{Kind: SourceQueue, Ref: "x"}}); err == nil {
		t.Error("a consumer with no name was accepted")
	}
	if err := r.Register(&testConsumer{name: "x", source: EventSource{Kind: SourceQueue}}); err == nil {
		t.Error("a consumer with no source was accepted")
	}
	if err := r.Register(&testConsumer{name: "x", source: EventSource{Ref: "y"}}); err == nil {
		t.Error("a consumer with no source kind was accepted")
	}
}

// The behaviour the whole dispatch design rests on: which consumer gets a batch
// is decided by the ARN, not by the order things were registered.
func TestMatchQueueARNRoutesByARNNotOrder(t *testing.T) {
	sms := queueConsumerFor("sms", "sms", true)
	email := queueConsumerFor("email", "email", true)
	r := newTestRegistry(t, sms, email)

	got, ok := r.MatchQueueARN("arn:aws:sqs:ap-southeast-1:123456789012:Ginboot-app1-env1-email")
	if !ok {
		t.Fatal("no consumer matched a queue that was registered")
	}
	if got.Name() != "email" {
		t.Fatalf("the email queue was routed to consumer %q", got.Name())
	}
}

func TestMatchQueueARNExternalRequiresExactARN(t *testing.T) {
	arn := "arn:aws:sqs:ap-southeast-1:123456789012:legacy-queue"
	external := queueConsumerFor("legacy", arn, false)
	r := newTestRegistry(t, external)

	if _, ok := r.MatchQueueARN(arn); !ok {
		t.Error("an external consumer did not match its own ARN")
	}
	// Same queue name, different region. Matching this would deliver another
	// region's traffic to this handler.
	if _, ok := r.MatchQueueARN("arn:aws:sqs:eu-west-1:123456789012:legacy-queue"); ok {
		t.Error("an external consumer matched a same-named queue in another region")
	}
}

func TestMatchQueueARNUnknownQueue(t *testing.T) {
	r := newTestRegistry(t, queueConsumerFor("sms", "sms", true))
	if _, ok := r.MatchQueueARN("arn:aws:sqs:ap-southeast-1:123456789012:something-else"); ok {
		t.Error("a queue nothing consumes was matched to a consumer")
	}
}

// A batch of records where one fails must report exactly that one. Reporting
// more redelivers work already done; reporting fewer loses the failure.
func TestDispatchReportsOnlyFailedRecords(t *testing.T) {
	c := queueConsumerFor("sms", "sms", true)
	boom := errors.New("downstream refused")
	c.fail["m2"] = boom
	r := newTestRegistry(t, c)

	result := r.Dispatch(context.Background(), c, []Event{
		{ID: "m1"}, {ID: "m2"}, {ID: "m3"},
	})

	if len(result.FailedIDs) != 1 || result.FailedIDs[0] != "m2" {
		t.Fatalf("expected only m2 to fail, got %v", result.FailedIDs)
	}
	if !errors.Is(result.Errors["m2"], boom) {
		t.Errorf("the failure recorded for m2 was %v", result.Errors["m2"])
	}
	if len(c.handled) != 3 {
		t.Errorf("a failing record stopped the batch: handled %v", c.handled)
	}
}

// A panic is one bad record, not a lost batch. If it escaped, records already
// handled would be redelivered and handled twice.
func TestDispatchContainsPanicToOneRecord(t *testing.T) {
	c := queueConsumerFor("sms", "sms", true)
	c.panicOn = "m2"
	r := newTestRegistry(t, c)

	result := r.Dispatch(context.Background(), c, []Event{{ID: "m1"}, {ID: "m2"}, {ID: "m3"}})

	if len(result.FailedIDs) != 1 || result.FailedIDs[0] != "m2" {
		t.Fatalf("expected only the panicking record to fail, got %v", result.FailedIDs)
	}
	if len(c.handled) != 2 {
		t.Errorf("records either side of the panic were not handled: %v", c.handled)
	}
}

// A cancelled context means the runtime is reclaiming the invocation. Anything
// not attempted must go back to the queue rather than be silently acknowledged.
func TestDispatchFailsUnattemptedRecordsWhenCancelled(t *testing.T) {
	c := queueConsumerFor("sms", "sms", true)
	r := newTestRegistry(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := r.Dispatch(ctx, c, []Event{{ID: "m1"}, {ID: "m2"}})
	if len(result.FailedIDs) != 2 {
		t.Fatalf("a cancelled invocation acknowledged records it never ran: %v", result.FailedIDs)
	}
	if len(c.handled) != 0 {
		t.Errorf("a handler ran under a cancelled context: %v", c.handled)
	}
}

func TestTypedQueueConsumerDecodesBody(t *testing.T) {
	type message struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}

	var got message
	c := NewQueueConsumer("sms", Queue("sms"), func(ctx context.Context, m message) error {
		got = m
		return nil
	})

	err := c.Handle(context.Background(), Event{ID: "m1", Body: []byte(`{"to":"+94771234567","text":"hi"}`)})
	if err != nil {
		t.Fatalf("handling a well-formed message failed: %v", err)
	}
	if got.To != "+94771234567" || got.Text != "hi" {
		t.Errorf("the message decoded as %+v", got)
	}
}

// An undecodable message must fail, so it reaches the dead letter queue rather
// than being deleted unread.
func TestTypedQueueConsumerFailsOnUndecodableBody(t *testing.T) {
	c := NewQueueConsumer("sms", Queue("sms"), func(ctx context.Context, m struct{}) error {
		t.Error("the handler ran on a message that could not be decoded")
		return nil
	})

	if err := c.Handle(context.Background(), Event{ID: "m1", Body: []byte(`not json`)}); err == nil {
		t.Fatal("an undecodable message was acknowledged as handled")
	}
}

func TestQueueURLReportsNotProvisioned(t *testing.T) {
	if _, err := QueueURL("nothing-declared-this"); !errors.Is(err, ErrQueueNotProvisioned) {
		t.Fatalf("expected ErrQueueNotProvisioned, got %v", err)
	}
}

func TestQueueURLReadsInjectedVariable(t *testing.T) {
	t.Setenv("GINBOOT_QUEUE_SMS_URL", "https://sqs.ap-southeast-1.amazonaws.com/123456789012/Ginboot-a-b-sms")
	url, err := QueueURL("sms")
	if err != nil {
		t.Fatalf("a provisioned queue reported as missing: %v", err)
	}
	if url == "" {
		t.Fatal("an empty URL was returned for a provisioned queue")
	}
}

// Once the platform has injected the real queue name, matching is exact rather
// than by suffix.
func TestMatchQueueARNPrefersProvisionedName(t *testing.T) {
	t.Setenv("GINBOOT_QUEUE_SMS_URL", "https://sqs.ap-southeast-1.amazonaws.com/123456789012/Ginboot-a-b-sms")
	c := queueConsumerFor("sms", "sms", true)
	r := newTestRegistry(t, c)

	if _, ok := r.MatchQueueARN("arn:aws:sqs:ap-southeast-1:123456789012:Ginboot-a-b-sms"); !ok {
		t.Fatal("the provisioned queue did not match its consumer")
	}
}

func TestManifestDescribesRegisteredTriggers(t *testing.T) {
	r := newTestRegistry(t,
		NewQueueConsumer("sms", Queue("sms"), func(ctx context.Context, m struct{}) error { return nil }),
		NewQueueConsumerWithBatchSize("bulk", FIFOQueue("bulk"), 5, func(ctx context.Context, m struct{}) error { return nil }),
	)

	manifest := r.Manifest()
	if len(manifest.Triggers) != 2 {
		t.Fatalf("expected 2 triggers, got %d", len(manifest.Triggers))
	}

	// Ordered by name, so the manifest is stable between reads.
	if manifest.Triggers[0].Name != "bulk" || manifest.Triggers[1].Name != "sms" {
		t.Errorf("the manifest was not ordered by name: %v", manifest.Triggers)
	}
	if manifest.Triggers[0].BatchSize != 5 || !manifest.Triggers[0].FIFO {
		t.Errorf("the FIFO trigger was described as %+v", manifest.Triggers[0])
	}
	if manifest.Triggers[1].BatchSize != DefaultBatchSize {
		t.Errorf("a trigger with no batch size was not given the default: %+v", manifest.Triggers[1])
	}
	for _, trigger := range manifest.Triggers {
		if !trigger.Deployable {
			t.Errorf("a well-formed trigger was reported undeployable: %+v", trigger)
		}
	}
}

// An external queue declared by name rather than ARN is the silent mistake this
// check exists to catch: it works locally and never receives anything deployed.
func TestManifestFlagsExternalQueueDeclaredByName(t *testing.T) {
	r := newTestRegistry(t, NewQueueConsumer("legacy", ExternalQueue("legacy-queue"),
		func(ctx context.Context, m struct{}) error { return nil }))

	manifest := r.Manifest()
	if manifest.Triggers[0].Deployable {
		t.Fatal("an external queue declared by bare name was reported deployable")
	}
	if manifest.Triggers[0].Problem == "" {
		t.Error("an undeployable trigger carried no explanation")
	}
}

func TestManifestFlagsUnusableManagedName(t *testing.T) {
	r := newTestRegistry(t, NewQueueConsumer("bad", Queue("has spaces!"),
		func(ctx context.Context, m struct{}) error { return nil }))

	if r.Manifest().Triggers[0].Deployable {
		t.Fatal("a managed name SQS would reject was reported deployable")
	}
}

func TestRegisterRejectsNil(t *testing.T) {
	r := NewConsumerRegistry(NewSlogLogger(nil))
	if err := r.Register(nil); err == nil {
		t.Fatal("a nil consumer was accepted")
	}
}

// A registry may hold consumers of several kinds. Queue matching must step over
// the ones that are not queues rather than inspect their refs as ARNs.
func TestMatchQueueARNIgnoresOtherSourceKinds(t *testing.T) {
	topic := &testConsumer{
		name:   "alerts",
		source: EventSource{Kind: SourceTopic, Ref: "arn:aws:sns:ap-southeast-1:123456789012:alerts", Managed: false},
		fail:   map[string]error{},
	}
	queue := queueConsumerFor("sms", "sms", true)
	r := newTestRegistry(t, topic, queue)

	got, ok := r.MatchQueueARN("arn:aws:sqs:ap-southeast-1:123456789012:Ginboot-a-b-sms")
	if !ok {
		t.Fatal("the queue consumer was not matched")
	}
	if got.Name() != "sms" {
		t.Fatalf("a queue was routed to the %q consumer", got.Name())
	}

	// And a topic ARN matches no queue consumer.
	if _, ok := r.MatchQueueARN("arn:aws:sns:ap-southeast-1:123456789012:alerts"); ok {
		t.Error("a topic ARN matched a queue consumer")
	}
}

func TestMustQueueURLReturnsAProvisionedURL(t *testing.T) {
	const url = "https://sqs.ap-southeast-1.amazonaws.com/123456789012/Ginboot-a-b-sms"
	t.Setenv("GINBOOT_QUEUE_SMS_URL", url)

	if got := MustQueueURL("sms"); got != url {
		t.Fatalf("MustQueueURL returned %q", got)
	}
}

// MustQueueURL is for a caller that would rather fail loudly than send into
// nothing. It must not return an empty string.
func TestMustQueueURLPanicsWhenNotProvisioned(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("MustQueueURL returned rather than panicking for an unprovisioned queue")
		}
		err, ok := recovered.(error)
		if !ok || !errors.Is(err, ErrQueueNotProvisioned) {
			t.Fatalf("the panic carried %v rather than ErrQueueNotProvisioned", recovered)
		}
	}()
	MustQueueURL("never-declared")
}

// A URL with no path separator still yields a name rather than an empty string,
// so a malformed injection degrades to the suffix match instead of matching
// everything.
func TestProvisionedQueueNameHandlesAURLWithNoPath(t *testing.T) {
	t.Setenv("GINBOOT_QUEUE_ODD_URL", "Ginboot-a-b-odd")
	if got := provisionedQueueName("odd"); got != "Ginboot-a-b-odd" {
		t.Fatalf("provisionedQueueName = %q", got)
	}
}

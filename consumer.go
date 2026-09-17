package ginboot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// An application is woken by three things: a request, a clock, and an event.
// This file is the third.
//
// It follows the scheduler's shape deliberately. What jobs an application has is
// a question only the running application can answer, so the platform asks it
// and remembers the answer — and what an application consumes is the same kind
// of question, with the same answer. Declaring triggers in a manifest file a
// build could read would make the declaration a second source of truth that can
// disagree with the code; asking the process cannot disagree with itself.
//
// The cost is identical too: a trigger is invisible until the deployment that
// introduces it is running, so the infrastructure behind it arrives one
// deployment later. That is reported rather than hidden. See TriggerManifest.

// EventSourceKind names the class of thing that produces an event.
//
// It exists so dispatch and the platform can talk about sources without either
// knowing the other's vocabulary: the framework recognises a payload and names
// its kind, and the platform reads that name and knows which infrastructure to
// provision. Adding a source is a constant here and a branch in the runner, not
// a change to either side's model.
type EventSourceKind string

const (
	// SourceQueue is SQS.
	SourceQueue EventSourceKind = "queue"
	// SourceTopic is SNS.
	SourceTopic EventSourceKind = "topic"
	// SourceBus is an EventBridge rule on a custom bus.
	SourceBus EventSourceKind = "bus"
	// SourceObjects is S3 object notifications.
	SourceObjects EventSourceKind = "objects"
	// SourceStream is a DynamoDB or Kinesis stream.
	SourceStream EventSourceKind = "stream"
)

// DefaultBatchSize is how many records a consumer is handed at once when it does
// not say.
//
// Ten, matching SQS's own default for a Lambda event source mapping. Larger
// batches amortise invocations but widen what one poison record can hold up, and
// ten is the number every piece of SQS documentation is written against.
const DefaultBatchSize = 10

// EventSource is what a consumer listens to.
type EventSource struct {
	Kind EventSourceKind

	// Ref names the source. When Managed it is a logical name the application
	// chose and the platform provisions under; otherwise it is the full ARN of
	// something that already exists.
	//
	// One field rather than two, because every consumer has exactly one of them
	// and a pair of fields where one is always empty invites code that reads the
	// wrong one and works until the day someone switches modes.
	Ref string

	// Managed is true when the platform owns the underlying resource: it creates
	// it, wires the trigger, and tells the application where it landed. False
	// means the application brought its own and the platform only subscribes.
	Managed bool

	// BatchSize is how many records to deliver per invocation. Zero means
	// DefaultBatchSize.
	BatchSize int

	// FIFO marks a first-in-first-out queue. It changes the resource the platform
	// creates and obliges every send to carry a message group, so it is declared
	// rather than detected — detection would only be possible after the queue
	// exists, which is one deployment too late to create it correctly.
	FIFO bool
}

// batchSize is BatchSize with the default applied.
func (s EventSource) batchSize() int {
	if s.BatchSize <= 0 {
		return DefaultBatchSize
	}
	return s.BatchSize
}

// Event is one record, whatever produced it.
//
// Deliberately not an SQS message. A consumer written against this keeps
// working when the same payload starts arriving from SNS or a bus, and the
// framework can add a source without changing a signature every application
// implements.
type Event struct {
	// ID is the source's own identifier for this record — an SQS message id, an
	// S3 event id. It is what a partial batch failure is reported against, so a
	// source that cannot supply one cannot report partial failures and fails its
	// batch whole.
	ID string

	// Body is the payload, already unwrapped from whatever envelope the source
	// put it in. An SNS notification delivered through SQS arrives here as the
	// message the publisher sent, not as the two envelopes it travelled in.
	Body []byte

	// Attributes are the source's own metadata: SQS message attributes, an S3
	// object key, a bus detail-type. Flattened to strings because that is what
	// every source agrees on and what a handler can branch on without a type
	// switch.
	Attributes map[string]string

	// Source is the consumer's declared source, so a handler shared between two
	// registrations can tell which one delivered this.
	Source EventSource

	// Raw is the record exactly as the source sent it, for the handler that needs
	// something this struct did not think to model.
	Raw json.RawMessage
}

// Consumer is one trigger an application declares.
type Consumer interface {
	// Name identifies the consumer to the platform and in telemetry. It is the
	// natural key the control plane records against, so it must be stable across
	// deployments — renaming one is removing a trigger and adding another.
	Name() string
	Source() EventSource
	Handle(ctx context.Context, ev Event) error
}

// QueueRef names an SQS queue, either one the platform should create or one that
// already exists.
type QueueRef struct {
	name    string
	arn     string
	fifo    bool
	managed bool
}

// Queue names a queue the platform provisions for this application.
//
// The name is logical and local to the application: the platform prefixes it
// with the application and environment, so two applications may both have a
// "sms" queue without collision, and the application never has to know the real
// name. Where it does need it — to send a message — it asks QueueURL.
func Queue(name string) QueueRef {
	return QueueRef{name: strings.TrimSpace(name), managed: true}
}

// FIFOQueue is Queue for a first-in-first-out queue.
//
// Separate from an option on Queue because it changes the contract on the
// sending side as well: every message needs a group id. A caller who has to
// name the constructor differently is a caller who has been told.
func FIFOQueue(name string) QueueRef {
	return QueueRef{name: strings.TrimSpace(name), managed: true, fifo: true}
}

// ExternalQueue names a queue that already exists, by ARN.
//
// The platform subscribes the application to it and grants the application
// access, but does not create, configure or delete it. An ARN rather than a name
// because a queue outside the platform's control may live in another region, and
// a bare name would silently resolve to the wrong one.
func ExternalQueue(arn string) QueueRef {
	return QueueRef{arn: strings.TrimSpace(arn)}
}

// source renders the reference as the EventSource the registry and manifest use.
func (q QueueRef) source(batchSize int) EventSource {
	ref := q.arn
	if q.managed {
		ref = q.name
	}
	return EventSource{
		Kind:      SourceQueue,
		Ref:       ref,
		Managed:   q.managed,
		BatchSize: batchSize,
		FIFO:      q.fifo,
	}
}

// queueConsumer is a typed handler over one queue.
type queueConsumer[T any] struct {
	name   string
	source EventSource
	handle func(ctx context.Context, msg T) error
}

func (c *queueConsumer[T]) Name() string        { return c.name }
func (c *queueConsumer[T]) Source() EventSource { return c.source }

func (c *queueConsumer[T]) Handle(ctx context.Context, ev Event) error {
	var msg T
	if err := json.Unmarshal(ev.Body, &msg); err != nil {
		// Returned rather than logged and swallowed, so the record fails its
		// batch and lands in the dead letter queue after the redrive policy gives
		// up. A message this consumer cannot read will never become readable by
		// being deleted quietly, and the copy in the DLQ is the only evidence of
		// what arrived.
		return fmt.Errorf("consumer %q could not read message %s: %w", c.name, ev.ID, err)
	}
	return c.handle(ctx, msg)
}

// NewQueueConsumer builds a consumer that decodes each message into T.
//
// This mirrors how a route handler binds a request body: the common case never
// touches the envelope. A consumer that does need the envelope — a message
// attribute, the raw bytes — implements Consumer directly and receives the Event.
func NewQueueConsumer[T any](name string, queue QueueRef, fn func(ctx context.Context, msg T) error) Consumer {
	return &queueConsumer[T]{
		name:   strings.TrimSpace(name),
		source: queue.source(0),
		handle: fn,
	}
}

// NewQueueConsumerWithBatchSize is NewQueueConsumer for a consumer that wants a
// batch size other than the default.
func NewQueueConsumerWithBatchSize[T any](name string, queue QueueRef, batchSize int, fn func(ctx context.Context, msg T) error) Consumer {
	return &queueConsumer[T]{
		name:   strings.TrimSpace(name),
		source: queue.source(batchSize),
		handle: fn,
	}
}

// ConsumerRegistry holds the consumers an application registered.
type ConsumerRegistry struct {
	mu        sync.RWMutex
	consumers map[string]Consumer
	logger    Logger
}

func NewConsumerRegistry(logger Logger) *ConsumerRegistry {
	return &ConsumerRegistry{
		consumers: make(map[string]Consumer),
		logger:    logger,
	}
}

// Register adds a consumer.
//
// A registration missing a name or a source is refused here, at startup, where
// whoever wrote it is still looking — rather than at the first event, which for
// a queue nobody has written to yet may be weeks away or never.
func (r *ConsumerRegistry) Register(c Consumer) error {
	if c == nil {
		return fmt.Errorf("cannot register a nil consumer")
	}
	name := strings.TrimSpace(c.Name())
	if name == "" {
		return fmt.Errorf("cannot register a consumer with no name")
	}
	source := c.Source()
	if strings.TrimSpace(source.Ref) == "" {
		return fmt.Errorf("consumer %q names no source to consume from", name)
	}
	if source.Kind == "" {
		return fmt.Errorf("consumer %q does not say what kind of source %q is", name, source.Ref)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.consumers[name]; exists {
		// Refused rather than replaced. Two consumers under one name is a
		// copy-paste away, and silently keeping the second means the first one's
		// messages go to a handler nobody expects.
		return fmt.Errorf("a consumer named %q is already registered", name)
	}
	r.consumers[name] = c
	return nil
}

// All returns every registered consumer, ordered by name so the manifest is
// stable between reads.
func (r *ConsumerRegistry) All() []Consumer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]Consumer, 0, len(r.consumers))
	for _, c := range r.consumers {
		all = append(all, c)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name() < all[j].Name() })
	return all
}

// ByName returns one consumer.
func (r *ConsumerRegistry) ByName(name string) (Consumer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.consumers[strings.TrimSpace(name)]
	return c, ok
}

// Len reports how many consumers are registered.
func (r *ConsumerRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.consumers)
}

// MatchQueueARN finds the consumer an SQS record belongs to.
//
// Matching is on the ARN the record carries, never on registration order. One
// function carries every mapping an application declared, and batches from
// different queues are indistinguishable by shape — so an ordinal match would
// route an email to the SMS handler the first time somebody reordered two
// registrations, and would do it silently.
//
// A managed queue's real name is assigned by the platform, not by the
// application, so the exact name is learned from the environment variable the
// platform injected. The suffix match behind it covers the deployment where that
// variable has not arrived yet, and any platform that prefixes differently.
func (r *ConsumerRegistry) MatchQueueARN(arn string) (Consumer, bool) {
	queueName := arn
	if idx := strings.LastIndex(arn, ":"); idx != -1 {
		queueName = arn[idx+1:]
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var suffixMatch Consumer
	for _, c := range r.consumers {
		source := c.Source()
		if source.Kind != SourceQueue {
			continue
		}

		if !source.Managed {
			// An external queue was declared by ARN, so compare ARNs and accept
			// nothing looser. A name-only match here could pull in a queue of the
			// same name in another region.
			if strings.EqualFold(strings.TrimSpace(source.Ref), strings.TrimSpace(arn)) {
				return c, true
			}
			continue
		}

		if provisioned := provisionedQueueName(source.Ref); provisioned != "" && provisioned == queueName {
			return c, true
		}
		if strings.HasSuffix(queueName, "-"+source.Ref) || queueName == source.Ref {
			// Held rather than returned, so an exact match later in the map still
			// wins. Map iteration has no order, and a fallback that returned first
			// would make which consumer handled a message depend on hash seed.
			suffixMatch = c
		}
	}

	if suffixMatch != nil {
		return suffixMatch, true
	}
	return nil, false
}

// ErrQueueNotProvisioned reports that a queue an application declared does not
// exist yet.
//
// This is the ordinary state on the deployment that introduces a consumer, not a
// fault. The platform learns about a trigger by asking the running application,
// which it can only do once the application is running — so the queue arrives
// one deployment later. An application that treated this as fatal could never
// reach the state that fixes it.
var ErrQueueNotProvisioned = fmt.Errorf("this queue has not been provisioned yet")

// queueURLEnv is the variable the platform injects for a managed queue.
func queueURLEnv(name string) string {
	upper := strings.ToUpper(strings.TrimSpace(name))
	upper = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(upper)
	return "GINBOOT_QUEUE_" + upper + "_URL"
}

// QueueURL returns the URL of a managed queue, for the sending side.
//
// It returns ErrQueueNotProvisioned rather than an empty string alone, because
// the two states a caller must tell apart — "not deployed yet" and "you asked
// for a queue you never declared" — are both an absence, and only one of them is
// fixed by deploying again.
func QueueURL(name string) (string, error) {
	url := strings.TrimSpace(os.Getenv(queueURLEnv(name)))
	if url == "" {
		return "", fmt.Errorf("%w: %q. It is created by the deployment after the one that first registered its consumer", ErrQueueNotProvisioned, name)
	}
	return url, nil
}

// MustQueueURL is QueueURL for a caller that has already checked, or that would
// rather fail loudly at startup than send into nothing.
//
// Not used on the path that boots the application. A panic here on the first
// deployment of a new consumer would stop the application before it could serve
// the manifest that gets the queue created — the deadlock ErrQueueNotProvisioned
// exists to avoid.
func MustQueueURL(name string) string {
	url, err := QueueURL(name)
	if err != nil {
		panic(err)
	}
	return url
}

// provisionedQueueName is the real name of a managed queue, read from the URL
// the platform injected, or empty before that has happened.
func provisionedQueueName(name string) string {
	url, err := QueueURL(name)
	if err != nil {
		return ""
	}
	if idx := strings.LastIndex(url, "/"); idx != -1 {
		return url[idx+1:]
	}
	return url
}

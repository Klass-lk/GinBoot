package lambda

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/klass-lk/ginboot"
)

// Routing an event to the consumer that declared it.
//
// One function carries every event source mapping an application declared, so
// the payload arriving here could belong to any of them. Every match below is on
// the ARN the record carries, never on registration order: batches from two
// queues are indistinguishable by shape, so an ordinal match would route one
// queue's messages to the other's handler the first time somebody reordered two
// registrations — and would do it silently, in production, under load.

// sqsEventResponse is returned for an SQS batch. Empty means every record
// succeeded.
type sqsEventResponse = events.SQSEventResponse

// handleSQS routes an SQS batch to its consumer and reports which records
// failed.
//
// The returned response names only the failures, which is what keeps a batch of
// ten with one bad message from redelivering the other nine. It only has that
// effect when the event source mapping declares ReportBatchItemFailures — the
// framework and the generated template have to agree on that, or this is
// computed carefully and then ignored.
func handleSQS(ctx context.Context, registry *ginboot.ConsumerRegistry, event events.SQSEvent) (sqsEventResponse, error) {
	response := sqsEventResponse{}
	if registry == nil || len(event.Records) == 0 {
		return response, nil
	}

	// Grouped by queue rather than assumed to be one. An event source mapping is
	// per queue, so in practice every record here shares an ARN — but grouping
	// costs one map and removes the need for that to stay true.
	byARN := make(map[string][]events.SQSMessage)
	order := make([]string, 0, 1)
	for _, record := range event.Records {
		if _, seen := byARN[record.EventSourceARN]; !seen {
			order = append(order, record.EventSourceARN)
		}
		byARN[record.EventSourceARN] = append(byARN[record.EventSourceARN], record)
	}

	for _, arn := range order {
		records := byARN[arn]

		consumer, found := registry.MatchQueueARN(arn)
		if !found {
			// Every record fails. The alternative — acknowledging them — deletes
			// messages nothing has read, and the queue that was configured to
			// deliver here goes quietly empty. Failing puts them in the dead
			// letter queue instead, where they can be replayed once the consumer
			// that should have had them is deployed.
			for _, record := range records {
				response.BatchItemFailures = append(response.BatchItemFailures,
					events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
			}
			continue
		}

		source := consumer.Source()
		batch := make([]ginboot.Event, 0, len(records))
		for _, record := range records {
			batch = append(batch, ginboot.Event{
				ID:         record.MessageId,
				Body:       unwrapSNSEnvelope([]byte(record.Body)),
				Attributes: sqsAttributes(record),
				Source:     source,
				Raw:        rawRecord(record),
			})
		}

		result := registry.Dispatch(ctx, consumer, batch)
		for _, id := range result.FailedIDs {
			response.BatchItemFailures = append(response.BatchItemFailures,
				events.SQSBatchItemFailure{ItemIdentifier: id})
		}
	}

	return response, nil
}

// snsEnvelope is the wrapper SNS puts around a message when a topic delivers to
// a queue.
type snsEnvelope struct {
	Type     string `json:"Type"`
	TopicArn string `json:"TopicArn"`
	Message  string `json:"Message"`
}

// unwrapSNSEnvelope returns the message a publisher actually sent.
//
// A topic subscribed to a queue delivers its notification as the queue message's
// body, so a consumer written against the published payload would otherwise have
// to know it was reading an envelope — and would break the day the same payload
// started arriving directly. Unwrapping here keeps the handler's type the thing
// the publisher sent.
//
// Recognised by all three fields together. "Type" alone is a plausible field
// name in an application's own messages, and unwrapping one of those would hand
// the handler an empty body.
func unwrapSNSEnvelope(body []byte) []byte {
	var envelope snsEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return body
	}
	if envelope.Type != "Notification" || envelope.TopicArn == "" || envelope.Message == "" {
		return body
	}
	return []byte(envelope.Message)
}

// sqsAttributes flattens what SQS carries alongside a message.
//
// The system attributes and the user's own message attributes are merged into
// one map, with the user's winning, because a handler asking for "TraceId" wants
// the one it set. Binary attributes are omitted: they have no string form, and a
// map of strings that sometimes holds base64 is worse than one that never does.
func sqsAttributes(record events.SQSMessage) map[string]string {
	attributes := make(map[string]string, len(record.Attributes)+len(record.MessageAttributes))
	for k, v := range record.Attributes {
		attributes[k] = v
	}
	for k, v := range record.MessageAttributes {
		if v.StringValue != nil {
			attributes[k] = *v.StringValue
		}
	}
	return attributes
}

// rawRecord re-renders one record for a handler that needs what this package did
// not model.
func rawRecord(record events.SQSMessage) json.RawMessage {
	// The error is discarded rather than checked. An SQSMessage is strings and
	// maps of strings all the way down, with nothing json.Marshal can refuse, so
	// a check here would be a branch no input can reach — and one that would sit
	// in every coverage report forever as a line nobody can test.
	encoded, _ := json.Marshal(record)
	return encoded
}

// looksLikeSQS reports whether a payload is an SQS batch.
//
// Checked on the decoded records rather than by searching the raw text, because
// "aws:sqs" appearing anywhere in a payload is not the same claim as a record
// declaring itself an SQS record — an API request carrying a queue ARN in its
// body would satisfy the second reading and be routed to a consumer.
func looksLikeSQS(raw []byte) (events.SQSEvent, bool) {
	var event events.SQSEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return event, false
	}
	if len(event.Records) == 0 {
		return event, false
	}
	for _, record := range event.Records {
		if !strings.EqualFold(record.EventSource, "aws:sqs") {
			return event, false
		}
	}
	return event, true
}

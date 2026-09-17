package ginboot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// What an application consumes is discoverable the same way its jobs are: by
// asking the process. See the note at the top of workers_route.go for why the
// declaration lives in the code rather than in a file a build could read, and
// what that costs.
//
// This is a second endpoint rather than a widening of the worker manifest.
// /_ginboot/workers has a published shape that the control plane and every
// already-deployed application agree on, and adding a field to it would mean a
// platform reading triggers from applications too old to report them — which is
// exactly the silent half-answer the manifest exists to replace. A separate path
// answers 404 on an old application, which is unambiguous.
const defaultTriggersPath = "/_ginboot/triggers"

// triggersPathEnv overrides where the trigger manifest is mounted. Access and
// token are shared with the worker manifest; only the path is separate, because
// only the path has to differ.
const triggersPathEnv = "GINBOOT_TRIGGERS_PATH"

// TriggerDescription is one registered consumer, as the process sees it.
type TriggerDescription struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
	// Managed says whether the platform owns the underlying resource. It decides
	// whether the platform creates a queue or merely subscribes to one, so it is
	// reported rather than inferred from the shape of Ref.
	Managed   bool `json:"managed"`
	BatchSize int  `json:"batchSize"`
	FIFO      bool `json:"fifo"`
	// Deployable is false when this trigger cannot be wired as declared.
	// Reported rather than omitted, for the reason WorkerDescription gives: a
	// trigger that exists and can never fire is a different problem from one
	// nobody wrote, and only one of them is fixed by writing code.
	Deployable bool `json:"deployable"`
	// Problem explains an undeployable trigger, and is empty otherwise.
	Problem string `json:"problem,omitempty"`
}

// TriggerManifest is what the endpoint answers with.
type TriggerManifest struct {
	Runtime  string               `json:"runtime"`
	Triggers []TriggerDescription `json:"triggers"`
}

// sqsARN matches a fully-qualified SQS queue ARN.
//
// Checked here, at registration, rather than at deploy time. An application that
// declared an external queue by name instead of by ARN — an easy and silent
// mistake, since both are strings and one of them works locally — otherwise gets
// a deployment that succeeds and a consumer that never receives anything.
var sqsARN = regexp.MustCompile(`^arn:[a-z0-9-]+:sqs:[a-z0-9-]+:\d{12}:[A-Za-z0-9_\-]+(\.fifo)?$`)

// managedRef constrains a logical name the platform builds a queue name from.
var managedRef = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// undeployable explains why a consumer cannot be wired as declared, or is empty.
func undeployable(c Consumer) string {
	source := c.Source()

	if source.Managed {
		// A managed name becomes part of a real queue name the platform builds,
		// and SQS accepts only these characters. Refusing here names the consumer;
		// refusing at CloudFormation names a resource id nobody wrote.
		if !managedRef.MatchString(source.Ref) {
			return fmt.Sprintf("the name %q can only contain letters, digits, hyphens and "+
				"underscores, because it becomes part of the queue name the platform creates",
				source.Ref)
		}
		return ""
	}

	if source.Kind == SourceQueue && !sqsARN.MatchString(source.Ref) {
		return "an external queue is named by its full ARN " +
			"(arn:aws:sqs:<region>:<account>:<queue>), not by its name — " +
			"a bare name cannot say which region or account the queue is in"
	}

	return ""
}

// Manifest describes every registered consumer.
func (r *ConsumerRegistry) Manifest() TriggerManifest {
	runtime := "server"
	if TickRuntime() {
		runtime = "tick"
	}

	consumers := r.All()
	triggers := make([]TriggerDescription, 0, len(consumers))
	for _, c := range consumers {
		source := c.Source()
		problem := undeployable(c)
		triggers = append(triggers, TriggerDescription{
			Name:       c.Name(),
			Kind:       string(source.Kind),
			Ref:        source.Ref,
			Managed:    source.Managed,
			BatchSize:  source.batchSize(),
			FIFO:       source.FIFO,
			Deployable: problem == "",
			Problem:    problem,
		})
	}

	return TriggerManifest{Runtime: runtime, Triggers: triggers}
}

// registerTriggersEndpoint mounts the trigger manifest. Called from Start,
// beside the worker manifest.
func (s *Server) registerTriggersEndpoint() {
	access, token, serve := manifestAccess("the trigger manifest")
	if !serve {
		return
	}

	path := manifestPath(os.Getenv(triggersPathEnv), defaultTriggersPath)

	consumers := s.consumers
	s.engine.GET(path, func(c *gin.Context) {
		if !authorizeManifest(c, access, token) {
			return
		}
		if consumers == nil {
			c.JSON(http.StatusOK, TriggerManifest{Triggers: []TriggerDescription{}})
			return
		}
		c.JSON(http.StatusOK, consumers.Manifest())
	})
}

// registerTriggerDeliveryEndpoint mounts a way to hand a consumer a payload by
// hand.
//
// A consumer that can only be exercised by deploying is a consumer nobody tests:
// the handler is ordinary code, but the only route to it runs through a queue in
// somebody's AWS account. This gives it a local one.
//
// Debug mode only, and never on a runtime that is woken from outside. The
// endpoint invokes application code with a caller-supplied payload, which is
// exactly what a queue does and exactly what nothing on the public internet
// should be able to do — so it is not gated on a token that could be
// misconfigured, it is absent from every build that is not a developer's own.
// EnsureAirConfig above uses the same condition for the same reason.
func (s *Server) registerTriggerDeliveryEndpoint() {
	if TickRuntime() || gin.Mode() != gin.DebugMode {
		return
	}

	access, token, serve := manifestAccess("the trigger delivery endpoint")
	if !serve {
		return
	}

	path := manifestPath(os.Getenv(triggersPathEnv), defaultTriggersPath)

	consumers := s.consumers
	s.engine.POST(path+"/:name", func(c *gin.Context) {
		if !authorizeManifest(c, access, token) {
			return
		}
		if consumers == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "no consumers are registered"})
			return
		}

		name := c.Param("name")
		consumer, found := consumers.ByName(name)
		if !found {
			// Names the consumers that do exist. The mistake this endpoint
			// produces most often is a typo in a name, and a bare 404 leaves
			// someone checking their registration code instead of their URL.
			registered := make([]string, 0, consumers.Len())
			for _, existing := range consumers.All() {
				registered = append(registered, existing.Name())
			}
			c.JSON(http.StatusNotFound, gin.H{
				"error":      fmt.Sprintf("no consumer named %q is registered", name),
				"registered": registered,
			})
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the request body could not be read"})
			return
		}

		event := Event{
			ID:     strings.TrimSpace(c.Query("id")),
			Body:   body,
			Source: consumer.Source(),
			Raw:    json.RawMessage(body),
		}
		if event.ID == "" {
			event.ID = "local-delivery"
		}

		// Through Dispatch rather than straight to Handle, so what happens here
		// is what happens in production: the same span, the same panic
		// containment, the same reporting of which record failed.
		result := consumers.Dispatch(c.Request.Context(), consumer, []Event{event})
		if len(result.FailedIDs) > 0 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"consumer": consumer.Name(),
				"outcome":  "failed",
				// The handler's own error, verbatim. In production this record
				// would now be redelivered and eventually land in the dead letter
				// queue; locally the message is the whole point.
				"error": result.Errors[event.ID].Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"consumer": consumer.Name(), "outcome": "handled"})
	})
}

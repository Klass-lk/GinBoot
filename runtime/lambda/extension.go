package lambda

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/klass-lk/ginboot"
)

// Telemetry is batched rather than exported as it is produced, because
// exporting inline puts a network round trip on the path of every span and log
// line. On a server the batch leaves a moment later and nobody notices. On
// Lambda nobody is running a moment later: the execution environment is frozen
// the instant the handler returns, and a frozen process exports nothing.
//
// Worse, the freeze does not pause the export's clock. An HTTP request suspended
// mid-flight still has a wall-clock deadline, so by the time the environment
// thaws — seconds or minutes later — the deadline has long passed and the export
// dies with "context deadline exceeded". A handler that returns in a couple of
// milliseconds never wins that race, and loses not some of its telemetry but all
// of it.
//
// The way out is the Extensions API. An extension registered for INVOKE gets to
// finish after the handler does: Lambda freezes the environment only once the
// runtime *and* every registered extension have asked for the next event. So the
// drain runs in a window that exists for exactly this purpose.
//
// What this costs, and what it does not:
//
//   - It does not delay the caller. Lambda returns the response as soon as the
//     runtime produces it, whether or not extensions are still working.
//   - It does extend the invocation. Billed duration covers the runtime plus its
//     extensions, so a drain that takes 200ms is 200ms of billed time on a
//     handler that ran for two. Watch PostRuntimeExtensionsDuration for the real
//     figure, and see GINBOOT_TELEMETRY_FLUSH_TIMEOUT to bound it.
//
// This registers from inside the runtime process, which Lambda calls an internal
// extension. Internal extensions may register for INVOKE only — asking for
// SHUTDOWN is rejected — which is why the drain hangs off the invoke event
// rather than off shutdown.
const (
	extensionAPIVersion = "2020-01-01"
	extensionName       = "ginboot-telemetry"

	// Long enough for a round trip to a collector on the far side of the world,
	// short enough that a collector that has stopped answering cannot eat the
	// function's timeout. Override with GINBOOT_TELEMETRY_FLUSH_TIMEOUT.
	defaultFlushTimeout = 2 * time.Second
)

// telemetryExtension drains telemetry in the window Lambda leaves open between
// the response going out and the environment freezing.
type telemetryExtension struct {
	id     string
	client *http.Client
	api    string

	// handlerDone carries one token per invocation. Buffered, and sent to
	// without blocking, so signalling it can never hold up a response — and so a
	// handler that finishes before this goroutine has even seen the invoke event
	// still leaves its token waiting.
	handlerDone chan struct{}

	flushTimeout time.Duration
}

// startTelemetryExtension registers with the Extensions API and starts draining.
// It returns nil when there is nothing to drain or the environment is not
// Lambda, and on any failure to register: telemetry is never a reason for a
// function not to serve, and every failure here leaves the runtime behaving
// exactly as it would without an extension.
func startTelemetryExtension() *telemetryExtension {
	api := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if api == "" {
		return nil
	}

	// No instrumentation compiled in means nothing to drain, and holding the
	// environment open to drain nothing would be billed time for no telemetry.
	if !ginboot.TelemetryBuffered() {
		return nil
	}

	extension := &telemetryExtension{
		api:          api,
		client:       &http.Client{},
		handlerDone:  make(chan struct{}, 1),
		flushTimeout: flushTimeout(),
	}

	if err := extension.register(); err != nil {
		fmt.Printf("[ginboot] telemetry will be exported on a best-effort basis: "+
			"could not register the Lambda extension: %v\n", err)
		return nil
	}

	go extension.run()
	return extension
}

// invocationComplete tells the extension the handler has finished and its
// response is on the way out. Never blocks.
func (e *telemetryExtension) invocationComplete() {
	if e == nil {
		return
	}
	select {
	case e.handlerDone <- struct{}{}:
	default:
	}
}

func (e *telemetryExtension) register() error {
	body := bytes.NewBufferString(`{"events":["INVOKE"]}`)
	url := fmt.Sprintf("http://%s/%s/extension/register", e.api, extensionAPIVersion)

	request, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	request.Header.Set("Lambda-Extension-Name", extensionName)
	request.Header.Set("Content-Type", "application/json")

	response, err := e.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("register returned %s", response.Status)
	}

	e.id = response.Header.Get("Lambda-Extension-Identifier")
	if e.id == "" {
		return fmt.Errorf("register returned no extension identifier")
	}
	return nil
}

// run is the extension's whole life: ask for an event, wait for the handler it
// belongs to, drain, ask again.
//
// Asking again is what tells Lambda this extension is finished, so it has to
// happen on every path through the loop. An iteration that returned without
// asking would leave the invoke phase open until the function's timeout —
// turning a telemetry problem into an outage — which is why the drain is wrapped
// rather than called inline.
func (e *telemetryExtension) run() {
	for {
		event, err := e.nextEvent()
		if err != nil {
			// The environment is going away, or the API has stopped answering.
			// Either way there is nothing useful left for this goroutine to do,
			// and retrying in a tight loop would only burn the CPU the function
			// is sharing with it.
			return
		}
		if event == shutdownEvent {
			return
		}

		e.awaitHandlerThenFlush()
	}
}

// awaitHandlerThenFlush waits for the invocation to finish and drains what it
// produced. It always returns, whatever the drain does.
func (e *telemetryExtension) awaitHandlerThenFlush() {
	defer func() {
		// A panic here would strand the invocation exactly as an early return
		// would, so it stops at this boundary.
		if recovered := recover(); recovered != nil {
			fmt.Printf("[ginboot] telemetry flush panicked: %v\n", recovered)
		}
	}()

	select {
	case <-e.handlerDone:
	case <-time.After(e.flushTimeout):
		// The handler is still running, or died without signalling. Draining now
		// is better than waiting on it: whatever it has already produced is
		// worth more than a tidy sequence, and Lambda is holding the invoke
		// phase open on this goroutine.
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.flushTimeout)
	defer cancel()

	if err := ginboot.FlushTelemetry(ctx); err != nil {
		fmt.Printf("[ginboot] telemetry flush incomplete: %v\n", err)
	}
}

const shutdownEvent = "SHUTDOWN"

// nextEvent blocks until Lambda has an event, and reports which kind it is.
//
// No timeout on this request, per the Extensions API: it is a long poll that
// Lambda answers when it has something to say, and the environment may be frozen
// for any length of time in between.
func (e *telemetryExtension) nextEvent() (string, error) {
	url := fmt.Sprintf("http://%s/%s/extension/event/next", e.api, extensionAPIVersion)

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Lambda-Extension-Identifier", e.id)

	response, err := e.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("event/next returned %s", response.Status)
	}

	// The body names the event type. Reading it is cheap and the payload is
	// small; an internal extension only ever sees INVOKE, but a body that says
	// otherwise is worth respecting rather than assuming.
	var payload struct {
		EventType string `json:"eventType"`
	}
	if err := decodeJSON(response.Body, &payload); err != nil {
		// An unreadable body is not a reason to abandon the loop — the event
		// still happened, and the handler still has telemetry to drain.
		return "INVOKE", nil
	}
	return payload.EventType, nil
}

func flushTimeout() time.Duration {
	raw := os.Getenv("GINBOOT_TELEMETRY_FLUSH_TIMEOUT")
	if raw == "" {
		return defaultFlushTimeout
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		// Milliseconds for a bare number, which is how the OTEL_* timeouts are
		// written and therefore what someone setting this will reach for.
		if millis, convErr := strconv.Atoi(raw); convErr == nil && millis > 0 {
			return time.Duration(millis) * time.Millisecond
		}
		return defaultFlushTimeout
	}
	return parsed
}

func decodeJSON(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

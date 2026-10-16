# Telemetry & Observability

`ginboot` provides a completely optional, pluggable telemetry module based on OpenTelemetry (OTLP). Because it is decoupled from the core framework, your application stays lightweight unless you explicitly opt-in to observability.

## Installation

```bash
go get github.com/klass-lk/ginboot/telemetry
```

## Setup

Import the package. The import is blank because you are not calling anything — you are compiling the plugin in so it can register itself with the framework:

```go
package main

import (
	"github.com/klass-lk/ginboot"
	_ "github.com/klass-lk/ginboot/telemetry"
)

func main() {
	server := ginboot.New()
	server.Start(8080)
}
```

With the plugin compiled in, either of these switches it on:

- `telemetry.enabled: true` in `ginboot.yml`, or
- an `OTEL_EXPORTER_OTLP_ENDPOINT` in the environment (or the signal-specific
  `..._TRACES_ENDPOINT`, `..._METRICS_ENDPOINT`, `..._LOGS_ENDPOINT`).

The second matters when a deployment ships a compiled binary and nothing else: there is no `ginboot.yml` to read at runtime, so `telemetry.enabled` would be `false` however the repository has it. If you want the file honoured on such a runtime, package it with the binary — `zip function.zip bootstrap ginboot.yml`.

With neither signal nothing is installed and telemetry costs nothing. To force it off where an environment names a collector you want no part of, set OpenTelemetry's own switch, which beats both:

```bash
OTEL_SDK_DISABLED=true
```

The telemetry module relies on the standard OpenTelemetry environment variables:
- `OTEL_EXPORTER_OTLP_ENDPOINT` (e.g. `https://otlp-gateway-prod-us-east-0.grafana.net/otlp`)
- `OTEL_EXPORTER_OTLP_HEADERS` (e.g. `Authorization=Basic <token>`)
- `OTEL_EXPORTER_OTLP_PROTOCOL` (defaults to `http/protobuf`)
- `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` — how much to record. Unset,
  everything is recorded and an upstream caller's decision not to sample is
  respected.

Values set in `ginboot.yml` are published as these variables, and a variable already present in the environment is left alone.

## Wiring it yourself

The import covers the common case. Call the API directly when you need a service name computed at runtime, your own logger, or payload capture turned on in code:

```go
package main

import (
	"context"
	"log"

	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/telemetry"
)

func main() {
	server := ginboot.New()

	// 1. Setup the OpenTelemetry Exporter (Connects to Grafana Cloud, Datadog, etc)
	shutdown, err := telemetry.Setup(context.Background(), "my-app-name", "v1.0.0")
	if err != nil {
		log.Printf("continuing without telemetry: %v", err)
	}
	defer shutdown(context.Background())

	// 2. Instrument the server with telemetry middlewares
	telemetry.Instrument(server, "my-app-name", nil)

	server.Start(8080)
}
```

Instrumenting twice is not additive — it would double every span, log line and metric — so the first caller wins and later ones do nothing. Mixing the blank import with an explicit `Instrument` call is safe.

Where a process exits deliberately, `server.Shutdown(ctx)` drains what is still buffered. It is not useful on a runtime that is suspended rather than stopped, such as AWS Lambda, which never gets far enough to run it.

## On AWS Lambda

Batching assumes the process is still running a moment later to send the batch. Lambda freezes the execution environment the instant the handler returns, and the export's wall-clock deadline keeps running while it is frozen — so a fast handler loses **all** of its telemetry to `context deadline exceeded`, not merely some of it.

The `runtime/lambda` runner handles this. It registers as a Lambda internal extension and drains telemetry in the window between the response going out and the environment freezing. There is nothing to configure.

This does not delay the caller: Lambda returns the response as soon as the handler produces it, whether or not extensions are still running. It does extend the invocation, and billed duration covers the runtime plus its extensions — on a handler that runs for 2ms, a 200ms drain is 200ms of billed time. Watch the `PostRuntimeExtensionsDuration` CloudWatch metric, and bound the drain with:

```bash
GINBOOT_TELEMETRY_FLUSH_TIMEOUT=500ms   # default 2s
```

None of this touches an HTTP server, which is never frozen. The machinery is in the `runtime/lambda` module, does nothing unless `AWS_LAMBDA_RUNTIME_API` is set, and does not register an extension when there is no telemetry to drain.

## Cost

Nothing here sits on the path of a request. Setting up exporters builds clients without dialing anything, and each record a request produces is handed to a batch processor that exports from its own goroutine — if its queue is full it drops rather than blocking your handler. Payload capture is off unless you ask for it.

## Database tracing

Every database call is a child span of the request that made it, so a slow
endpoint's trace shows which query took the time. ginboot's own
constructors install it; telemetry being off makes it nearly free (see
below).

| Store | Hook | Installed by | Your own client |
|---|---|---|---|
| MongoDB | `db/mongo.TracingMonitor` (command monitor) | `MongoConfig.Connect` | `options.Client().SetMonitor(mongo.TracingMonitor(nil))` |
| SQL via gorm (Postgres, MySQL, SQLite…) | `db/sql.TracingPlugin` (gorm plugin) | `SQLConfig.ConnectGORM` | `db.Use(sql.TracingPlugin{})` |
| DynamoDB | `db/dynamodb.WithTracing` (SDK middleware) | `NewDynamoDBClient` | `dynamodb.NewFromConfig(cfg, ginbootdynamo.WithTracing)` |

Pass the request's context down: a `*ginboot.Context` is a
`context.Context` whose `Value` sees the request's span, so
`repo.FindById(ctx, id)` from a handler is enough. For gorm use
`db.WithContext(ctx)`. A call made with `context.Background()` starts a
trace of its own.

Monitors chain. Put a guard outermost so a refused command never opens a
span: `mongo.ReadOnlyMonitor(mongo.TracingMonitor(nil))`.

### What a span holds

- Name `<operation> <collection or table>`: `find sessions`, `select users`,
  `GetItem users`. Calls over several tables (`BatchGetItem`,
  `TransactWriteItems`) have no table.
- `db.system`, `db.collection.name`, `db.operation.name`, and
  `db.namespace` (MongoDB database) or `db.response.returned_rows` (SQL).
- A failed call is marked as an error with a **code only**: the MongoDB
  error code name (`DuplicateKey`), the SQLSTATE (`SQLSTATE 23505`) or
  gorm error, or the DynamoDB error code
  (`ConditionalCheckFailedException`).

Never recorded: command bodies, filters, keys, items, statement arguments,
and error messages, because all of them can carry user data (a duplicate
key error quotes the value). `TracingPlugin{QueryText: true}` adds the
parameterised SQL (`... WHERE email = ?`), never its arguments. It is
opt-in because raw SQL that inlines values would carry them.

### Does it slow requests down?

Not measurably. The hook runs on the calling goroutine, but it only builds
a span. Export happens in the batch processor's own goroutine, and if the
processor's queue is full, spans are dropped. A query never waits on the
tracing backend. A span that will not be kept (telemetry off, or sampled
out) is neither tracked nor described.

`BenchmarkTracingMonitor` in `db/mongo` measures one command's
start and finish on an Apple M1:

| Situation | Added per call |
|---|---|
| Telemetry off (no-op provider) | ~0.25 µs |
| Sampled out | ~0.4 µs |
| Recorded, batch exporter | ~1.4 µs |

A MongoDB round trip takes 300–5000 µs, so even a recorded span adds under
0.5%. Under load, sampling (`OTEL_TRACES_SAMPLER_ARG`) brings the cost
down to the sampled-out figure.

### Tracing another store

Use the client library's own extension point, never a wrapper around every
call: a command monitor, a plugin or callback, a driver middleware. Then:

1. Start a `SpanKindClient` span from the call's context, named
   `<operation> <table>`.
2. If `!span.IsRecording()`, do nothing more.
3. Use the attribute names above, and record error codes, not messages.
4. Install it in ginboot's constructor for that store.
5. Test it with `tracetest.NewSpanRecorder()`: parenting, names, the error
   code, and that a value placed in the request never shows up in an
   attribute. Add a benchmark.

## Context-Bound Logger

`ginboot` comes with a powerful `Logger` interface bound directly to the request context. This ensures that any log you write from your business logic is automatically correlated with the current distributed trace!

Inside your controllers or services, just use `ctx.Logger()`:

```go
func (c *UserController) GetUser(ctx *ginboot.Context) (interface{}, error) {
    // 🪄 Magic: This log automatically gets a trace_id attached to it!
    ctx.Logger().Info("Fetching user from database", "user_id", 123)
    
    // ...
}
```

### Customizing the Logger

By default, the telemetry plugin configures a "Tee Logger" that prints human-readable logs to your local terminal while silently shipping structured logs to your OTLP backend (like Grafana Loki) in the background.

If you ever want to override this behavior and use your own logger (e.g., writing to a file), you can implement the `ginboot.Logger` interface and inject it into the server:

```go
server.SetLogger(myCustomFileLogger)
```

# Telemetry & Observability

`ginboot` provides a completely optional, pluggable telemetry module based on OpenTelemetry (OTLP). Because it is decoupled from the core framework, your application stays lightweight unless you explicitly opt-in to observability.

## Installation

```bash
go get github.com/klass-lk/ginboot/telemetry
```

## Setup

To enable distributed tracing, metrics, and remote logging, simply import the `telemetry` package and wire it into your `ginboot.Server`.

```go
package main

import (
	"context"
	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/telemetry"
)

func main() {
	server := ginboot.New()

	// 1. Setup the OpenTelemetry Exporter (Connects to Grafana Cloud, Datadog, etc)
	shutdown, _ := telemetry.Setup(context.Background(), "my-app-name", "v1.0.0")
	defer shutdown(context.Background())

	// 2. Instrument the server with telemetry middlewares
	telemetry.Instrument(server, "my-app-name", nil)

	server.Start(8080)
}
```

The telemetry module automatically relies on standard OpenTelemetry environment variables:
- `OTEL_EXPORTER_OTLP_ENDPOINT` (e.g. `https://otlp-gateway-prod-us-east-0.grafana.net/otlp`)
- `OTEL_EXPORTER_OTLP_HEADERS` (e.g. `Authorization=Basic <token>`)
- `OTEL_EXPORTER_OTLP_PROTOCOL` (defaults to `http/protobuf`)

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

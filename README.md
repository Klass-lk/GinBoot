# Ginboot Framework

A lightweight and powerful Go web framework built on top of Gin, designed for building scalable web applications with MongoDB integration and AWS Lambda support.

## Setup

### Prerequisites
- Go 1.21 or later
- MongoDB (for local development)
- AWS SAM CLI (for deployment)
- AWS credentials configured

### Installation

1. Install the Ginboot CLI tool:
```bash
go install github.com/klass-lk/ginboot-cli@latest
```

2. Create a new project:
```bash
# Create a new project
ginboot new myproject

# Navigate to project directory
cd myproject

# Initialize dependencies
go mod tidy
```

3. Run locally:
```bash
go run main.go
```
Your API will be available at `http://localhost:8080/api/v1`

### Build and Deploy

To deploy your application to AWS Lambda:

```bash
# Build the project for AWS Lambda
ginboot build

# Deploy to AWS
ginboot deploy
```

On first deployment, you'll be prompted for:
- Stack name (defaults to project name)
- AWS Region
- S3 bucket configuration

These settings will be saved in `ginboot-app.yml` for future deployments.

## Features

- **Database Operations**: Built-in multi-database support (MongoDB, SQL, DynamoDB) through a generic repository interface, enabling common CRUD operations with minimal code.
- **Pluggable Telemetry**: Optional, lightweight OpenTelemetry plugin (`ginboot/telemetry`) to ship traces, metrics, and logs straight to Grafana (or any OTLP backend) without bloating the core framework.
- **Context-Bound Logger**: A pluggable, context-aware logger (`ctx.Logger().Info(...)`) that automatically correlates logs with active distributed traces.
- **API Request Handling**: Simplified API request and authentication context extraction.
- **Error Handling**: Easily define and manage business errors.
- **Password Encoding**: Inbuilt password hashing and matching utility for secure authentication.
- **CORS Configuration**: Flexible CORS setup with both default and custom configurations.

## Installation

To install GinBoot, add it to your project:

```bash
go get github.com/klass-lk/ginboot
```

To use the optional telemetry plugin:
```bash
go get github.com/klass-lk/ginboot/telemetry
```

Then import it. The import is blank because you are not calling anything — it
compiles the plugin in so it can register itself with the framework:
```go
import _ "github.com/klass-lk/ginboot/telemetry"
```
From there, `telemetry.enabled: true` in `ginboot.yml` or an
`OTEL_EXPORTER_OTLP_ENDPOINT` in the environment switches it on. See
[Telemetry & Observability](docs/telemetry.md).

## Migrating an existing API to Ginboot

Already have an API? You do not need a rewrite.

- **Go apps migrate in place.** `server.Engine()` is the real `*gin.Engine` and Ginboot
  route groups accept Gin's own `func(c *gin.Context)` handlers, so your existing routes
  keep serving traffic while you convert one controller at a time.
- **APIs in other languages are ported against their contract** — Express, NestJS,
  FastAPI, Django, Spring Boot, ASP.NET, Rails or Laravel — then traffic moves path by
  path with both services running.

| Guide | |
| :--- | :--- |
| Start here | [Migrating to Ginboot](docs/migration.md) · [ginboot.com/docs/5-migration](https://ginboot.com/docs/5-migration) |
| From an existing Go app | [ginboot.com/docs/5-migration/from-go](https://ginboot.com/docs/5-migration/from-go) |
| From another language | [ginboot.com/docs/5-migration/from-other-languages](https://ginboot.com/docs/5-migration/from-other-languages) |
| For AI coding agents | [ginboot.com/docs/5-migration/agent-playbook](https://ginboot.com/docs/5-migration/agent-playbook) |

Using an AI coding agent? Point it at
[`AGENTS.md`](AGENTS.md), [ginboot.com/llms.txt](https://ginboot.com/llms.txt) or
[ginboot.com/llms-full.txt](https://ginboot.com/llms-full.txt) — the whole documentation
in one request.

## Documentation

For more detailed information on Ginboot's features and usage, refer to the following documentation:

*   [Server Configuration](docs/server.md)
*   [Routing](docs/routing.md)
*   [Authentication](docs/authentication.md)
*   [Database Support](docs/database.md)
*   [Deployment to AWS Lambda using SAM](docs/deployment.md)
*   [Testing](docs/testing.md)
*   [Caching Support](docs/caching.md)
*   [Telemetry & Observability](docs/telemetry.md)
*   [Migrating to Ginboot](docs/migration.md)

## Contributing
Contributions are welcome! Please read our contributing guidelines for more details.

## License
This project is licensed under the MIT License. See the LICENSE file for details.
#
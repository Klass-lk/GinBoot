# AGENTS.md — Ginboot for AI coding agents

Ginboot is a Go web framework built on [Gin](https://github.com/gin-gonic/gin):
controller-based routing, database-agnostic repositories, declarative configuration,
background workers and queue consumers, OpenTelemetry, and AWS Lambda support.

```bash
go get -u github.com/klass-lk/ginboot
```

## Where the documentation is

| What you need | Where |
| :--- | :--- |
| Index of every page | https://ginboot.com/llms.txt |
| The whole documentation in one request | https://ginboot.com/llms-full.txt |
| A single page as Markdown | `https://ginboot.com/llms.mdx/docs/<path>/content.md` |
| Package reference | https://pkg.go.dev/github.com/klass-lk/ginboot |

## Migrating an existing API to Ginboot

Read the guide that matches the task **before** editing code:

| Task | Guide |
| :--- | :--- |
| Migrate an existing **Go** app (Gin, net/http, Echo, Fiber, Chi, gorilla/mux) | https://ginboot.com/docs/5-migration/from-go |
| Port an API from **another language** (Express, NestJS, FastAPI, Django, Spring Boot, ASP.NET, Rails, Laravel) | https://ginboot.com/docs/5-migration/from-other-languages |
| **Step-by-step procedure for agents**, with the exact API surface and verification commands | https://ginboot.com/docs/5-migration/agent-playbook |
| Overview and strategy | https://ginboot.com/docs/5-migration |

Markdown source of the playbook, for direct fetching:
`https://ginboot.com/llms.mdx/docs/5-migration/agent-playbook/content.md`

## The short version

A Go app migrates **in place** and keeps running throughout: `ginboot.New()` takes over
the entrypoint, `server.Engine()` is the real `*gin.Engine` so existing routes mount
unchanged, and Ginboot route groups accept Gin's own `func(c *gin.Context)` handlers.
Convert one resource at a time; verify each step before the next.

An API in another language is **ported against its contract** — capture the endpoints,
request and response shapes and status codes first, build the new service against the
same database, then move traffic path by path behind a proxy with both services running.

```go
package main

import (
	"log"

	"github.com/klass-lk/ginboot"
)

type UserController struct{ users *service.UserService }

func (c *UserController) Register(group *ginboot.ControllerGroup) {
	group.GET("/:id", c.Get)
	group.POST("", c.Create)
}

// Handlers return (value, error). Ginboot binds the request, serialises the
// response and maps the error to a status code.
func (c *UserController) Get(ctx *ginboot.Context) (model.User, error) {
	return c.users.FindById(ctx.Param("id"))
}

func (c *UserController) Create(req CreateUserRequest) (model.User, error) {
	return c.users.Create(req)
}

func main() {
	server := ginboot.New()
	cfg := server.Config()

	server.SetBasePath(cfg.Ginboot.Server.BasePath)
	server.RegisterController("/users", userController)

	log.Fatal(server.Start(cfg.Ginboot.Server.Port))
}
```

## Rules that prevent the common mistakes

- **Server-wide middleware is `server.Engine().Use(...)`.** There is no `server.Use`, no
  `server.SetRuntime` and no `ginboot.RuntimeLambda`.
- **Repositories live in submodules**, not the root package:
  `github.com/klass-lk/ginboot/db/{mongo,sql,dynamodb,inmemory}`. There is no
  `ginboot.NewMongoRepository`.
- **Handler signatures** are `func(ctx *ginboot.Context) (T, error)`,
  `func(req R) (T, error)`, `func(ctx *ginboot.Context, req R) (T, error)`,
  `func() (T, error)`, or raw `func(c *gin.Context)`. With two parameters the context
  comes first. Anything else panics at registration.
- **Middleware comes after the handler**: `group.GET(path, handler, middleware...)`.
- **Success is always `200`.** For `201` or `204`, write the response yourself and return
  `nil, nil` — Ginboot leaves an already-written response alone.
- **Errors**: return `ginboot.NewApiError(404, "...")`. Anything that is not an
  `ApiError` becomes a `500`.
- **`ctx.GetAuthContext()` reads the Gin keys `user_id` and `role`** — auth middleware
  must set both.
- **`/healthz`, `/health`, panic recovery and OpenAPI generation are provided.** Do not
  re-implement them.
- **Verify, do not assume.** `go build ./... && go vet ./... && go test ./...` after every
  step, and check an uncertain API with `go doc github.com/klass-lk/ginboot.Server`
  rather than from memory.

## Working on this repository

This repo *is* the framework. `go test ./...` at the root, and in each submodule under
`db/`, `runtime/` and `telemetry/`, which are separate Go modules. The documentation site
is `docs-site/` (Next.js + Fumadocs); pages are MDX under `docs-site/content/docs/`, and
adding one puts it in the sitemap and in `llms.txt` automatically.

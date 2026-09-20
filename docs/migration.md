# Migrating to Ginboot

How to move an API you already have onto Ginboot — whether it is written in Go or in
something else.

The full guides live on the documentation site, which is also published in an
agent-readable form:

| Guide | URL |
| :--- | :--- |
| Overview and strategy | https://ginboot.com/docs/5-migration |
| Migrating an existing **Go** app (Gin, net/http, Echo, Fiber, Chi, gorilla/mux) | https://ginboot.com/docs/5-migration/from-go |
| Porting an API from **another language** (Express, NestJS, FastAPI, Django, Spring Boot, ASP.NET, Rails, Laravel) | https://ginboot.com/docs/5-migration/from-other-languages |
| **Playbook for AI coding agents** | https://ginboot.com/docs/5-migration/agent-playbook |
| Every page as one text file | https://ginboot.com/llms-full.txt |

## Which strategy applies

| Your API today | Strategy | Downtime |
| :--- | :--- | :--- |
| Go + Gin | **In place.** Swap the entrypoint, keep every route, convert one controller at a time. | None |
| Go + another router | **In place**, with handlers rewritten. Services, models and tests carry over. | None |
| Another language | **Port against the contract**, then move traffic path by path with both services running. | None |

Ginboot is Gin underneath and does not hide it: `server.Engine()` returns the real
`*gin.Engine`, and a Ginboot route group accepts Gin's own `func(c *gin.Context)`
handlers unchanged. That is what makes a Go migration incremental rather than a rewrite.

## The phases

1. **Take over the entrypoint.** `ginboot.New()` owns the process; existing routes are
   mounted on `server.Engine()`. Nothing else changes.
2. **Move configuration** into `ginboot.yml` and read it through `server.Config()`,
   keeping the environment variable names you already deploy with.
3. **Convert one endpoint group at a time** into a controller, and ship each one.
4. **Move the data access layer** onto a repository from the matching `db/*` module.
5. **Turn on the platform features** — telemetry, caching, OpenAPI, workers, consumers,
   Lambda — in whatever order operations needs.

The application builds, starts and serves traffic after every phase. Verify each one
before starting the next.

## What changes in the code

| Before | After |
| :--- | :--- |
| `ctx.ShouldBindJSON(&req)` then a 400 | A request struct parameter with `binding` tags |
| `ctx.JSON(200, v)` | `return v, nil` |
| `ctx.JSON(404, gin.H{"error": ...})` | `return ginboot.NewApiError(404, "...").New(id)` |
| Hand-parsed `page`/`size`/`sort` | `ctx.GetPageRequest()` |
| `os.Getenv` scattered through the code | `ginboot.yml` + `server.Config()` |
| Hand-written CRUD | `GenericRepository[T]` from `db/mongo`, `db/sql`, `db/dynamodb` or `db/inmemory` |
| A goroutine with a `time.Ticker` | `server.RegisterWorker(name, interval, fn)` |
| A queue polling loop | `server.RegisterConsumer(ginboot.NewQueueConsumer(...))` |
| `req.user` / session lookups in handlers | `ctx.GetAuthContext()` |

## Things that catch people out

- **Server-wide middleware is `server.Engine().Use(...)`.** There is no `server.Use`.
- **Repositories live in submodules**, not the root package — `go get
  github.com/klass-lk/ginboot/db/mongo` and import it.
- **Middleware comes after the handler**: `group.GET(path, handler, middleware...)`.
- **Success is always `200`.** For `201` or `204`, write the response yourself and return
  `nil, nil`; Ginboot leaves an already-written response alone.
- **`ctx.GetAuthContext()` reads the Gin keys `user_id` and `role`** — your auth
  middleware must set both, or every protected route returns `401`.
- **Watch the base path.** `SetBasePath("/api/v1")` prefixes every route; if your route
  strings already contain it you will get `/api/v1/api/v1/...`.
- **Schema migrations stay yours.** Ginboot does not manage database schema.

## Checklist

- [ ] `ginboot.New()` owns the entrypoint and the app starts.
- [ ] Settings come from `ginboot.yml` / the environment via `server.Config()`.
- [ ] Every route is registered inside a controller.
- [ ] Handlers return `(T, error)` and do not also write a response.
- [ ] Every error path returns a `ginboot.ApiError` with the status code the old API used.
- [ ] Auth middleware sets `user_id` and `role`.
- [ ] Background jobs are registered workers or consumers — running in exactly one service.
- [ ] `GINBOOT_EXPORT_SWAGGER=openapi.json go run .` matches the contract clients hold.
- [ ] The old routing, binding, error-formatting and config code is deleted.

## If you are an AI coding agent

Read https://ginboot.com/docs/5-migration/agent-playbook first, or fetch it directly as
Markdown:

```
https://ginboot.com/llms.mdx/docs/5-migration/agent-playbook/content.md
```

It states the exact API surface, the step order with a verification command after each
step, and the APIs that no longer exist but still appear in older material. See also
[AGENTS.md](../AGENTS.md) in the repository root.

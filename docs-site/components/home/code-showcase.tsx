import { highlight } from 'fumadocs-core/highlight';
import { CodeBlock, Pre } from 'fumadocs-ui/components/codeblock';
import { shikiThemes } from '@/lib/shiki';

/**
 * Condensed from `example/cmd/main.go` and
 * `example/internal/controller/post_controller.go`. Kept short enough to read
 * at a glance — the full versions live in the repo's `example/` directory.
 */
const samples = [
  {
    file: 'main.go',
    code: `package main

import (
	"log"

	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/controller"
)

func main() {
	// Loads ginboot.yml / application.yml and .env automatically
	server := ginboot.New()
	cfg := server.Config()

	server.SetBasePath(cfg.Ginboot.Server.BasePath)
	server.RegisterController("/posts", controller.NewPostController(svc))

	// Runs as HTTP locally, as an API Gateway proxy on AWS Lambda
	if err := server.Start(cfg.Ginboot.Server.Port); err != nil {
		log.Fatal(err)
	}
}`,
  },
  {
    file: 'post_controller.go',
    code: `package controller

import "github.com/klass-lk/ginboot"

func (c *PostController) Register(group *ginboot.ControllerGroup) {
	group.GET("/:id", c.GetPost)

	protected := group.Group("")
	protected.POST("", c.CreatePost)
}

// Handlers return (DTO, error) — Ginboot handles binding,
// status codes and JSON serialisation for you.
func (c *PostController) GetPost(ctx *ginboot.Context) (model.Post, error) {
	return c.postService.GetPostById(ctx.Param("id"))
}

func (c *PostController) CreatePost(ctx *ginboot.Context, req model.Post) (model.Post, error) {
	post, err := c.postService.CreatePost(req)
	if err != nil {
		return model.Post{}, ginboot.NewApiError(400, "invalid post")
	}

	// Fire-and-forget call to another service, traced end to end
	_ = ctx.CallServiceAsync("notification-service", "/notifications/send", post)

	return post, nil
}`,
  },
];

/** Highlighted on the server so there's no flash of unstyled code on load. */
async function Sample({ file, code }: { file: string; code: string }) {
  const rendered = await highlight(code, {
    lang: 'go',
    themes: shikiThemes,
    components: {
      pre: (props) => <Pre {...props} />,
    },
  });

  return (
    <CodeBlock title={file} className="my-0 h-full">
      {rendered}
    </CodeBlock>
  );
}

export function CodeShowcase() {
  return (
    <section className="mx-auto max-w-6xl px-4 py-20">
      <div className="mx-auto mb-12 max-w-2xl text-center">
        <h2 className="text-3xl font-bold tracking-tight sm:text-4xl">
          Controllers, not boilerplate
        </h2>
        <p className="mt-4 text-fd-muted-foreground">
          Register a controller and return typed DTOs. Ginboot wires up binding, error handling,
          tracing and serialisation — and the same code runs as an HTTP server or a Lambda function.
        </p>
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-2">
        {samples.map((sample) => (
          <Sample key={sample.file} {...sample} />
        ))}
      </div>
    </section>
  );
}

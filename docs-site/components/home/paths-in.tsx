import Link from 'next/link';
import { ArrowRight, Code2, Repeat } from 'lucide-react';

/**
 * Two audiences arrive with opposite worries: Go developers expect a framework
 * to hide Go behind magic, and developers from Spring, NestJS or Django expect
 * to lose the structure they rely on. Each card answers one of them.
 */
const paths = [
  {
    icon: Code2,
    eyebrow: 'Already writing Go',
    title: "It's still Gin underneath",
    body: 'server.Engine() returns the real *gin.Engine. Your Gin middleware and func(c *gin.Context) handlers work as they are, so you can adopt Ginboot one route group at a time and stop wherever you like.',
    cta: 'Migrate from Gin, Echo, Chi or net/http',
    href: '/docs/5-migration/from-go',
  },
  {
    icon: Repeat,
    eyebrow: 'Coming from Spring, NestJS or Django',
    title: 'The structure you know, in plain Go',
    body: "Controllers, repositories and a YAML config file sit where you'd expect them. The wiring is ordinary Go: you call constructors, return errors and can read the whole app top to bottom in main.go.",
    cta: 'Port an API from another language',
    href: '/docs/5-migration/from-other-languages',
  },
];

export function PathsIn() {
  return (
    <section className="mx-auto max-w-6xl px-4 py-20" aria-labelledby="paths-heading">
      <div className="mx-auto mb-12 max-w-2xl text-center">
        <h2 id="paths-heading" className="text-3xl font-bold tracking-tight sm:text-4xl">
          Familiar from either side
        </h2>
        <p className="mt-4 text-fd-muted-foreground">
          Ginboot adds structure without hiding Go. Nothing is generated and nothing is injected
          behind your back, so a stack trace leads to code you wrote.
        </p>
      </div>

      <div className="grid gap-5 md:grid-cols-2">
        {paths.map(({ icon: Icon, eyebrow, title, body, cta, href }) => (
          <Link key={href} href={href} className="glass-card glass-card-hover group flex flex-col p-8">
            <div className="mb-5 flex items-center gap-3">
              <div className="inline-flex rounded-lg bg-brand/10 p-2.5 text-brand ring-1 ring-brand/20">
                <Icon className="size-5" />
              </div>
              <span className="text-sm font-medium text-fd-muted-foreground">{eyebrow}</span>
            </div>
            <h3 className="mb-3 text-xl font-semibold group-hover:text-brand">{title}</h3>
            <p className="flex-1 leading-relaxed text-fd-muted-foreground">{body}</p>
            <span className="mt-6 inline-flex items-center gap-1.5 text-sm font-semibold text-brand">
              {cta}
              <ArrowRight className="size-4 transition-transform group-hover:translate-x-0.5" />
            </span>
          </Link>
        ))}
      </div>
    </section>
  );
}

import Link from 'next/link';
import { ArrowRight } from 'lucide-react';
import { GithubIcon } from '@/components/icons';
import { externalLinks } from '@/lib/shared';
import { InstallCommand } from './install-command';

export function Hero() {
  return (
    <section className="surface-grid relative overflow-hidden border-b border-fd-border">
      {/* Local glow, stronger than the ambient one on <body> */}
      <div
        aria-hidden
        className="pointer-events-none absolute -top-40 left-1/2 size-[42rem] -translate-x-1/2 rounded-full bg-brand/15 blur-[120px]"
      />

      <div className="relative mx-auto flex max-w-5xl flex-col items-center px-4 py-24 text-center sm:py-32">
        <Link
          href="/docs/4-advanced/changelog"
          className="glass-card animate-fade-in mb-8 inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium text-fd-muted-foreground transition-colors hover:text-fd-foreground"
        >
          <span className="rounded-full bg-brand/15 px-2 py-0.5 font-semibold text-brand">
            v1.1.0
          </span>
          Declarative config, service calls & hot reload
          <ArrowRight className="size-3" />
        </Link>

        <h1 className="animate-slide-up text-5xl font-extrabold tracking-tight text-balance sm:text-7xl">
          <span className="text-gradient-primary">Spring Boot</span>
          <span className="text-fd-foreground">, for Go.</span>
        </h1>

        <p className="animate-slide-up mt-6 max-w-2xl text-lg text-balance text-fd-muted-foreground sm:text-xl">
          Ginboot is an enterprise-ready Go web framework built on Gin. Database-agnostic
          repositories, AWS Lambda, OpenTelemetry and declarative configuration — all out of the box.
        </p>

        <div className="animate-slide-up mt-10 flex flex-wrap items-center justify-center gap-3">
          <Link
            href="/docs/1-getting-started"
            className="btn-glow inline-flex items-center gap-2 rounded-full bg-brand px-7 py-3 font-semibold text-brand-foreground"
          >
            Get Started
            <ArrowRight className="size-4" />
          </Link>
          <Link
            href={externalLinks.github}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-2 rounded-full border border-fd-border bg-fd-secondary px-7 py-3 font-semibold text-fd-secondary-foreground transition-colors hover:bg-fd-accent"
          >
            <GithubIcon className="size-4" />
            View on GitHub
          </Link>
        </div>

        <div className="animate-slide-up mt-10 flex justify-center">
          <InstallCommand command="go get -u github.com/klass-lk/ginboot" />
        </div>
      </div>
    </section>
  );
}

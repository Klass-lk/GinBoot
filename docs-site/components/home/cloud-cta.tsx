import Link from 'next/link';
import { ArrowUpRight, Rocket } from 'lucide-react';
import { externalLinks } from '@/lib/shared';

/**
 * Orange-leaning on purpose: the body of the page is cyan, so the commercial
 * band reads as a distinct surface rather than another feature section.
 */
export function CloudCta() {
  return (
    <section className="mx-auto max-w-6xl px-4 pb-24">
      <div className="glass-card relative overflow-hidden p-10 text-center sm:p-14">
        <div
          aria-hidden
          className="pointer-events-none absolute -right-24 -bottom-24 size-80 rounded-full bg-brand-orange/15 blur-[100px]"
        />
        <div
          aria-hidden
          className="pointer-events-none absolute -top-24 -left-24 size-80 rounded-full bg-brand/10 blur-[100px]"
        />

        <div className="relative">
          <div className="mb-5 inline-flex rounded-lg bg-brand-orange/10 p-2.5 text-brand-orange ring-1 ring-brand-orange/20">
            <Rocket className="size-5" />
          </div>
          <h2 className="text-3xl font-bold tracking-tight text-balance sm:text-4xl">
            Ship it with Ginboot Cloud
          </h2>
          <p className="mx-auto mt-4 max-w-xl text-balance text-fd-muted-foreground">
            One-click deploys, managed environments, live log streaming and telemetry dashboards —
            purpose-built for Ginboot services.
          </p>

          <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
            <Link
              href={externalLinks.cloud}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 rounded-full bg-brand-orange px-7 py-3 font-semibold text-white transition-transform hover:-translate-y-0.5"
            >
              Explore Ginboot Cloud
              <ArrowUpRight className="size-4" />
            </Link>
            <Link
              href={externalLinks.initializer}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-2 rounded-full border border-fd-border bg-fd-secondary px-7 py-3 font-semibold text-fd-secondary-foreground transition-colors hover:bg-fd-accent"
            >
              Generate a project
            </Link>
          </div>
        </div>
      </div>
    </section>
  );
}

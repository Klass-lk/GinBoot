import Link from 'next/link';
import { ArrowUpRight, Check, Rocket } from 'lucide-react';
import { externalLinks } from '@/lib/shared';

/**
 * Claims here mirror cloud.ginboot.com — keep the two in sync when pricing or
 * features change there.
 */
const points = [
  'Every push builds once. Test it in preprod, then promote that exact build to production',
  'Request, error and latency metrics from API Gateway and Lambda, with no instrumentation',
  'OpenTelemetry logs and traces from your Ginboot app, per request',
  'Deploy, promote and roll back from Claude Code, Cursor or VS Code over MCP',
];

const plans = [
  {
    name: 'Hosted',
    price: '$0',
    unit: 'platform fee',
    detail: 'Runs in our AWS account. A monthly allowance is included, then you pay for usage.',
  },
  {
    name: 'Your AWS',
    price: '$49',
    unit: '/ month per org',
    detail: 'Runs in your own account and AWS bills you directly. No Ginboot charges on traffic.',
  },
];

/**
 * Orange-leaning on purpose: the body of the page is cyan, so the commercial
 * band reads as a distinct surface rather than another feature section.
 */
export function CloudCta() {
  return (
    <section className="mx-auto max-w-6xl px-4 pb-24">
      <div className="glass-card relative overflow-hidden p-8 sm:p-14">
        <div
          aria-hidden
          className="pointer-events-none absolute -right-24 -bottom-24 size-80 rounded-full bg-brand-orange/15 blur-[100px]"
        />
        <div
          aria-hidden
          className="pointer-events-none absolute -top-24 -left-24 size-80 rounded-full bg-brand/10 blur-[100px]"
        />

        <div className="relative grid gap-12 lg:grid-cols-[1.2fr_1fr] lg:items-center">
          <div>
            <div className="mb-5 inline-flex items-center gap-2 rounded-full bg-brand-orange/10 px-3 py-1 text-sm font-medium text-brand-orange ring-1 ring-brand-orange/20">
              <Rocket className="size-4" />
              Ginboot Cloud
            </div>
            <h2 className="text-3xl font-bold tracking-tight text-balance sm:text-4xl">
              Push to GitHub. Run on AWS Lambda.
            </h2>
            <p className="mt-4 max-w-xl text-balance text-fd-muted-foreground">
              Your Ginboot service already runs on Lambda. Ginboot Cloud takes care of the rest:
              builds, environments, API Gateway and dashboards. You don't write any Terraform or
              YAML pipelines.
            </p>

            <ul className="mt-6 space-y-3">
              {points.map((point) => (
                <li key={point} className="flex gap-3 text-sm leading-relaxed">
                  <Check className="mt-0.5 size-4 shrink-0 text-brand-orange" />
                  <span>{point}</span>
                </li>
              ))}
            </ul>

            <div className="mt-8 flex flex-wrap items-center gap-3">
              <Link
                href={externalLinks.cloud}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-2 rounded-full bg-brand-orange px-7 py-3 font-semibold text-white transition-transform hover:-translate-y-0.5"
              >
                Deploy free
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

          <div className="grid gap-4">
            {plans.map(({ name, price, unit, detail }) => (
              <div key={name} className="rounded-xl border border-fd-border bg-fd-background/60 p-6">
                <div className="text-sm font-medium text-fd-muted-foreground">{name}</div>
                <div className="mt-1 flex items-baseline gap-2">
                  <span className="text-3xl font-bold">{price}</span>
                  <span className="text-sm text-fd-muted-foreground">{unit}</span>
                </div>
                <p className="mt-2 text-sm leading-relaxed text-fd-muted-foreground">{detail}</p>
              </div>
            ))}
            <p className="text-center text-xs text-fd-muted-foreground">
              No AWS account needed to start. Connect yours whenever you like.
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}

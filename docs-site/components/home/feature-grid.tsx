import Link from 'next/link';
import {
  Cloud,
  Database,
  FlaskConical,
  Network,
  Settings2,
  Telescope,
  type LucideIcon,
} from 'lucide-react';

const features: {
  icon: LucideIcon;
  title: string;
  description: string;
  href: string;
}[] = [
  {
    icon: Database,
    title: 'Database agnostic',
    description:
      'Generic repositories for MongoDB, SQL and DynamoDB behind one interface. CRUD and pagination with almost no code.',
    href: '/docs/3-features/database',
  },
  {
    icon: Cloud,
    title: 'Serverless ready',
    description:
      'Detects AWS Lambda at runtime and proxies API Gateway requests automatically. The same controllers run either way.',
    href: '/docs/3-features/aws-lambda',
  },
  {
    icon: Telescope,
    title: 'Built-in telemetry',
    description:
      'An optional OpenTelemetry plugin ships traces, metrics and logs to Grafana or any OTLP backend, with trace-correlated logging.',
    href: '/docs/3-features/telemetry',
  },
  {
    icon: Settings2,
    title: 'Declarative config',
    description:
      'ginboot.yml plus automatic .env loading and four env-var injection syntaxes. Read it back with a typed server.Config().',
    href: '/docs/2-core-concepts/configuration',
  },
  {
    icon: Network,
    title: 'Service to service',
    description:
      'ctx.CallService resolves targets from config, env or DNS and propagates W3C trace and auth headers for you.',
    href: '/docs/3-features/service-communication',
  },
  {
    icon: FlaskConical,
    title: 'BDD testing',
    description:
      'Godog and Cucumber integration with built-in step definitions, so API behaviour is specified in plain Gherkin.',
    href: '/docs/4-advanced/testing',
  },
];

export function FeatureGrid() {
  return (
    <section className="mx-auto max-w-6xl px-4 py-20" aria-labelledby="features-heading">
      <div className="mx-auto mb-14 max-w-2xl text-center">
        <h2 id="features-heading" className="text-3xl font-bold tracking-tight sm:text-4xl">
          Batteries included
        </h2>
        <p className="mt-4 text-fd-muted-foreground">
          Everything a production Go service needs, wired up and ready — without giving up the Gin
          APIs you already know.
        </p>
      </div>

      <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
        {features.map(({ icon: Icon, title, description, href }) => (
          <Link key={title} href={href} className="glass-card glass-card-hover group p-6">
            <div className="mb-4 inline-flex rounded-lg bg-brand/10 p-2.5 text-brand ring-1 ring-brand/20">
              <Icon className="size-5" />
            </div>
            <h3 className="mb-2 font-semibold group-hover:text-brand">{title}</h3>
            <p className="text-sm leading-relaxed text-fd-muted-foreground">{description}</p>
          </Link>
        ))}
      </div>
    </section>
  );
}

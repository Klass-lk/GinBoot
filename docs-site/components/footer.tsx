import Link from 'next/link';
import { Logo } from '@/components/logo';
import { appName, externalLinks } from '@/lib/shared';

const columns = [
  {
    heading: 'Documentation',
    links: [
      { text: 'Getting Started', href: '/docs/1-getting-started' },
      { text: 'Core Concepts', href: '/docs/2-core-concepts/server' },
      { text: 'Features', href: '/docs/3-features/database' },
      { text: 'Deployment', href: '/docs/4-advanced/deployment' },
    ],
  },
  {
    heading: 'Product',
    links: [
      { text: 'Initializer', href: externalLinks.initializer, external: true },
      { text: 'Ginboot Cloud', href: externalLinks.cloud, external: true },
      { text: 'Changelog', href: '/docs/4-advanced/changelog' },
    ],
  },
  {
    heading: 'Community',
    links: [
      { text: 'GitHub', href: externalLinks.github, external: true },
      { text: 'Issues', href: `${externalLinks.github}/issues`, external: true },
      { text: 'Releases', href: `${externalLinks.github}/releases`, external: true },
    ],
  },
];

export function Footer() {
  return (
    <footer className="border-t border-fd-border bg-fd-card/30">
      <div className="mx-auto max-w-6xl px-4 py-14">
        <div className="grid gap-10 sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <Logo />
            <p className="mt-3 max-w-xs text-sm leading-relaxed text-fd-muted-foreground">
              An enterprise-ready Go web framework built on Gin.
            </p>
          </div>

          {columns.map((column) => (
            <div key={column.heading}>
              <h3 className="mb-3 text-xs font-semibold tracking-wider text-fd-foreground uppercase">
                {column.heading}
              </h3>
              <ul className="space-y-2.5">
                {column.links.map((link) => (
                  <li key={link.text}>
                    <Link
                      href={link.href}
                      {...(link.external ? { target: '_blank', rel: 'noreferrer' } : {})}
                      className="text-sm text-fd-muted-foreground transition-colors hover:text-brand"
                    >
                      {link.text}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        <div className="mt-12 flex flex-col items-center justify-between gap-3 border-t border-fd-border pt-6 text-sm text-fd-muted-foreground sm:flex-row">
          <p>
            © {new Date().getFullYear()} {appName}. Released under the MIT License.
          </p>
          <p>
            Built with{' '}
            <Link href={externalLinks.github} target="_blank" rel="noreferrer" className="hover:text-brand">
              Ginboot
            </Link>
          </p>
        </div>
      </div>
    </footer>
  );
}

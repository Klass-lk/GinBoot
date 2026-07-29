import type { ReactNode } from 'react';

/**
 * Renders one of Ginboot's supported handler shapes with a plain-language note
 * about when to reach for it. Used by `2-core-concepts/routing.mdx`.
 */
export function ApiSignature({
  signature,
  children,
}: {
  signature: string;
  children: ReactNode;
}) {
  return (
    <div className="not-prose my-3 overflow-hidden rounded-lg border border-fd-border bg-fd-card">
      <code className="block overflow-x-auto border-b border-fd-border bg-fd-secondary/50 px-4 py-2.5 font-mono text-[13px] whitespace-nowrap text-fd-foreground">
        {signature}
      </code>
      <div className="px-4 py-3 text-sm leading-relaxed text-fd-muted-foreground">{children}</div>
    </div>
  );
}

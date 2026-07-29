import Link from 'next/link';
import { ArrowUpRight } from 'lucide-react';
import { externalLinks } from '@/lib/shared';

/** Pinned to the bottom of the docs sidebar. */
export function SidebarCta() {
  return (
    <Link
      href={externalLinks.initializer}
      target="_blank"
      rel="noreferrer"
      className="glass-card glass-card-hover group block p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-[13px] font-semibold">Start a project</span>
        <ArrowUpRight className="size-3.5 text-fd-muted-foreground transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
      </div>
      <p className="mt-1 text-xs leading-relaxed text-fd-muted-foreground">
        Scaffold a Ginboot app in seconds with start.ginboot.com
      </p>
    </Link>
  );
}

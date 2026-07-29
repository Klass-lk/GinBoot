import { source } from '@/lib/source';
import { DocsLayout } from 'fumadocs-ui/layouts/notebook';
import { BookOpen, Boxes, Layers, Rocket, Settings2 } from 'lucide-react';
import type { ReactNode } from 'react';
import { baseOptions } from '@/lib/layout.shared';
import { SidebarCta } from '@/components/sidebar-cta';

// Keyed by the folder name, which is also the first URL segment.
const sectionIcons: Record<string, ReactNode> = {
  '1-getting-started': <Rocket />,
  '2-core-concepts': <Layers />,
  '3-features': <Boxes />,
  '4-advanced': <Settings2 />,
};

export default function Layout({ children }: LayoutProps<'/docs'>) {
  const base = baseOptions();

  return (
    <DocsLayout
      tree={source.getPageTree()}
      {...base}
      nav={{ ...base.nav, mode: 'top' }}
      tabMode="navbar"
      tabs={{
        transform: (tab, node) => {
          const folder = node.$id?.split('/').at(-1) ?? '';

          return { ...tab, icon: sectionIcons[folder] ?? <BookOpen /> };
        },
      }}
      sidebar={{ collapsible: true, footer: <SidebarCta /> }}
    >
      {children}
    </DocsLayout>
  );
}

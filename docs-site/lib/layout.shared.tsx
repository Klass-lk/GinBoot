import type { BaseLayoutProps } from 'fumadocs-ui/layouts/shared';
import { BookOpen, Cloud, Rocket } from 'lucide-react';
import { Logo } from '@/components/logo';
import { docsRoute, externalLinks } from './shared';

export function baseOptions(): BaseLayoutProps {
  return {
    nav: {
      title: <Logo />,
      url: '/',
    },
    githubUrl: externalLinks.github,
    links: [
      {
        text: 'Documentation',
        url: docsRoute,
        active: 'nested-url',
        icon: <BookOpen />,
      },
      {
        text: 'Initializer',
        url: externalLinks.initializer,
        external: true,
        icon: <Rocket />,
      },
      {
        text: 'Ginboot Cloud',
        url: externalLinks.cloud,
        external: true,
        icon: <Cloud />,
      },
    ],
  };
}

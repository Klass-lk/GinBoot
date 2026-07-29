import type { MetadataRoute } from 'next';
import { source } from '@/lib/source';
import { siteUrl } from '@/lib/shared';

export const revalidate = false;

export default function sitemap(): MetadataRoute.Sitemap {
  const absolute = (path: string) => new URL(path, siteUrl).toString();

  return [
    {
      url: absolute('/'),
      changeFrequency: 'weekly',
      priority: 1,
    },
    ...source.getPages().map((page) => ({
      url: absolute(page.url),
      changeFrequency: 'weekly' as const,
      // The docs root outranks individual pages, but all docs sit below the
      // landing page.
      priority: page.slugs.length === 0 ? 0.8 : 0.6,
    })),
  ];
}

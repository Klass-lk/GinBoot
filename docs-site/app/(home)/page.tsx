import { Hero } from '@/components/home/hero';
import { CodeShowcase } from '@/components/home/code-showcase';
import { FeatureGrid } from '@/components/home/feature-grid';
import { PathsIn } from '@/components/home/paths-in';
import { CloudCta } from '@/components/home/cloud-cta';
import type { Metadata } from 'next';
import { appName, externalLinks, siteUrl } from '@/lib/shared';

export const metadata: Metadata = {
  alternates: {
    canonical: '/',
  },
};

const jsonLd = {
  '@context': 'https://schema.org',
  '@type': 'SoftwareSourceCode',
  name: appName,
  url: siteUrl,
  codeRepository: externalLinks.github,
  programmingLanguage: 'Go',
  license: 'https://opensource.org/licenses/MIT',
  description:
    'Ginboot is an open-source Go web framework built on Gin. It handles request binding, error mapping, configuration, database repositories, OpenTelemetry tracing and AWS Lambda, while handlers stay plain Go functions.',
  keywords: [
    'Go web framework',
    'Golang API framework',
    'serverless Go',
    'AWS Lambda Go framework',
    'Golang microservices',
  ],
};

export default function HomePage() {
  return (
    <>
      {/* eslint-disable-next-line react/no-danger */}
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
      />
      <Hero />
      <CodeShowcase />
      <PathsIn />
      <FeatureGrid />
      <CloudCta />
    </>
  );
}

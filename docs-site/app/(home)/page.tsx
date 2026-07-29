import { Hero } from '@/components/home/hero';
import { CodeShowcase } from '@/components/home/code-showcase';
import { FeatureGrid } from '@/components/home/feature-grid';
import { CloudCta } from '@/components/home/cloud-cta';
import { appName, externalLinks, siteUrl } from '@/lib/shared';

const jsonLd = {
  '@context': 'https://schema.org',
  '@type': 'SoftwareSourceCode',
  name: appName,
  url: siteUrl,
  codeRepository: externalLinks.github,
  programmingLanguage: 'Go',
  license: 'https://opensource.org/licenses/MIT',
  description:
    'Ginboot is an enterprise-ready, high-performance Go web framework built on top of Gin, with database-agnostic repositories, AWS Lambda support, OpenTelemetry and declarative configuration.',
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
      <FeatureGrid />
      <CloudCta />
    </>
  );
}

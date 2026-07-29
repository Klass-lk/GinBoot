import { ImageResponse } from 'next/og';
import { OgTemplate } from '@/components/og-template';
import { appTagline } from '@/lib/shared';

export const alt = 'Ginboot — the Go web framework for modern APIs';
export const size = { width: 1200, height: 630 };
export const contentType = 'image/png';

export default function Image() {
  return new ImageResponse(
    <OgTemplate
      title={appTagline}
      description="An enterprise-ready Go web framework built on Gin — database-agnostic repositories, AWS Lambda, and OpenTelemetry out of the box."
    />,
    size,
  );
}

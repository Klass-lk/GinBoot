import { ImageResponse } from 'next/og';
import { OgTemplate } from '@/components/og-template';
import { appTagline } from '@/lib/shared';

export const alt = 'Ginboot: open-source Go web framework built on Gin';
export const size = { width: 1200, height: 630 };
export const contentType = 'image/png';

export default function Image() {
  return new ImageResponse(
    <OgTemplate
      title={appTagline}
      description="An open-source Go web framework on Gin. Binding, config, repositories, OpenTelemetry and AWS Lambda built in, with handlers that stay plain Go."
    />,
    size,
  );
}

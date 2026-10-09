import type { Metadata } from 'next';
import { RootProvider } from 'fumadocs-ui/provider/next';
import './global.css';
import { Inter, JetBrains_Mono } from 'next/font/google';
import { siteUrl } from '@/lib/shared';

// Exposed as CSS variables rather than `className` so `--font-sans` /
// `--font-mono` in global.css can pick them up — otherwise Tailwind's
// `font-sans` and the fumadocs typography plugin fall back to the system stack.
const inter = Inter({
  subsets: ['latin'],
  variable: '--font-inter',
  display: 'swap',
});

const jetbrainsMono = JetBrains_Mono({
  subsets: ['latin'],
  variable: '--font-jetbrains-mono',
  display: 'swap',
});

export const metadata: Metadata = {
  metadataBase: new URL(siteUrl),
  title: {
    default: 'Ginboot: Open-Source Go Web Framework Built on Gin',
    template: '%s | Ginboot',
  },
  description:
    'Build production REST APIs in Go on top of Gin. Ginboot handles binding, errors, config, SQL, MongoDB and DynamoDB repositories, OpenTelemetry and AWS Lambda.',
  keywords: [
    'Go web framework',
    'Golang web framework',
    'Gin framework',
    'Go REST API framework',
    'Golang REST API',
    'Go microservices framework',
    'Go AWS Lambda',
    'serverless Go',
    'OpenTelemetry Go',
    'Go repository pattern',
  ],
  openGraph: {
    title: 'Ginboot: Gin, with the boring parts done',
    description:
      'An open-source Go web framework on Gin. Binding, errors, config, repositories and tracing come built in, and your handlers stay plain Go functions.',
    url: siteUrl,
    siteName: 'Ginboot',
    locale: 'en_US',
    type: 'website',
  },
  twitter: {
    card: 'summary_large_image',
    title: 'Ginboot: Gin, with the boring parts done',
    description:
      'Open-source Go web framework on Gin. Binding, config, repositories, OpenTelemetry and AWS Lambda built in. No annotations, no DI container.',
  },
  robots: {
    index: true,
    follow: true,
  },
};

export default function Layout({ children }: LayoutProps<'/'>) {
  return (
    <html
      lang="en"
      className={`${inter.variable} ${jetbrainsMono.variable}`}
      suppressHydrationWarning
    >
      <body className="flex min-h-screen flex-col font-sans">
        <RootProvider>{children}</RootProvider>
      </body>
    </html>
  );
}

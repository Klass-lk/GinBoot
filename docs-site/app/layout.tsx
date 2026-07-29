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
    default: 'Ginboot - The Best Go Web Framework for Modern APIs',
    template: '%s | Ginboot',
  },
  description:
    'Ginboot is an enterprise-ready, high-performance Golang web framework built on top of Gin. Features out-of-the-box MongoDB, SQL, DynamoDB support, AWS Lambda serverless execution, OpenTelemetry, and BDD testing.',
  keywords: [
    'Go web framework',
    'Golang API framework',
    'Gin web framework',
    'Golang REST API',
    'serverless Go',
    'AWS Lambda Go framework',
    'Golang microservices',
    'best Go framework',
  ],
  openGraph: {
    title: 'Ginboot | The Ultimate Golang Web Framework',
    description:
      'Build robust, scalable APIs in Go faster than ever. Built-in DB repositories, AWS Lambda support, and Telemetry.',
    url: siteUrl,
    siteName: 'Ginboot',
    type: 'website',
  },
  twitter: {
    card: 'summary_large_image',
    title: 'Ginboot | High-Performance Go Framework',
    description: 'The easiest way to build enterprise Go microservices. Try Ginboot today.',
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

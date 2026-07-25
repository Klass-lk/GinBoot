import type { Metadata } from 'next';
import { RootProvider } from 'fumadocs-ui/provider/next';
import './global.css';
import { Inter } from 'next/font/google';

const inter = Inter({
  subsets: ['latin'],
});

export const metadata: Metadata = {
  title: 'Ginboot - The Best Go Web Framework for Modern APIs',
  description: 'Ginboot is an enterprise-ready, high-performance Golang web framework built on top of Gin. Features out-of-the-box MongoDB, SQL, DynamoDB support, AWS Lambda serverless execution, OpenTelemetry, and BDD testing.',
  keywords: ['Go web framework', 'Golang API framework', 'Gin web framework', 'Golang REST API', 'serverless Go', 'AWS Lambda Go framework', 'Golang microservices', 'best Go framework'],
  openGraph: {
    title: 'Ginboot | The Ultimate Golang Web Framework',
    description: 'Build robust, scalable APIs in Go faster than ever. Built-in DB repositories, AWS Lambda support, and Telemetry.',
    url: 'https://ginboot.com',
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
  }
};

export default function Layout({ children }: LayoutProps<'/'>) {
  return (
    <html lang="en" className={inter.className} suppressHydrationWarning>
      <body className="flex flex-col min-h-screen">
        <RootProvider>{children}</RootProvider>
      </body>
    </html>
  );
}

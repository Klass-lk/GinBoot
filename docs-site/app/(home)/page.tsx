import Link from 'next/link';

export default function HomePage() {
  return (
    <div className="flex flex-col flex-1 items-center justify-center text-center px-4 py-20 mt-10">
      <div className="relative">
        <div className="absolute inset-0 bg-purple-500/20 blur-[100px] rounded-full" />
        <h1 className="text-5xl md:text-7xl font-extrabold tracking-tight mb-6 bg-gradient-to-r from-purple-500 to-indigo-500 text-transparent bg-clip-text relative z-10">
          Ginboot
        </h1>
      </div>
      
      <p className="text-xl md:text-2xl text-fd-muted-foreground max-w-2xl mb-12">
        A lightweight and powerful Go web framework built on top of Gin, designed for building scalable web applications with MongoDB integration and AWS Lambda support.
      </p>

      <div className="flex flex-wrap items-center justify-center gap-4">
        <Link 
          href="/docs" 
          className="bg-purple-600 hover:bg-purple-700 text-white font-semibold py-3 px-8 rounded-full transition-all shadow-lg hover:shadow-purple-500/25"
        >
          Get Started
        </Link>
        <Link 
          href="https://github.com/klass-lk/ginboot" 
          target="_blank"
          className="bg-fd-secondary hover:bg-fd-secondary/80 text-fd-foreground font-semibold py-3 px-8 rounded-full transition-all shadow-sm"
        >
          View on GitHub
        </Link>
      </div>

      <div className="mt-24 grid grid-cols-1 md:grid-cols-3 gap-8 max-w-5xl w-full text-left">
        <div className="p-6 border border-fd-border rounded-2xl bg-fd-card">
          <h3 className="text-xl font-bold mb-2 text-fd-foreground">Database Agnostic</h3>
          <p className="text-fd-muted-foreground">Built-in generic repositories for MongoDB, SQL, and DynamoDB allowing instant CRUD operations.</p>
        </div>
        <div className="p-6 border border-fd-border rounded-2xl bg-fd-card">
          <h3 className="text-xl font-bold mb-2 text-fd-foreground">AWS Lambda Ready</h3>
          <p className="text-fd-muted-foreground">Automatically detects AWS Lambda environment and seamlessly proxies API Gateway requests.</p>
        </div>
        <div className="p-6 border border-fd-border rounded-2xl bg-fd-card">
          <h3 className="text-xl font-bold mb-2 text-fd-foreground">Pluggable Telemetry</h3>
          <p className="text-fd-muted-foreground">Lightweight OpenTelemetry plugin to ship traces, metrics, and logs straight to Grafana without bloating the core framework.</p>
        </div>
      </div>
    </div>
  );
}

import Link from 'next/link';

export default function HomePage() {
  return (
    <main className="flex flex-col flex-1 items-center justify-center text-center px-4 py-20 mt-10" itemScope itemType="http://schema.org/SoftwareSourceCode">
      <header className="relative">
        <div className="absolute inset-0 bg-blue-500/20 blur-[100px] rounded-full" />
        <h1 className="text-5xl md:text-7xl font-extrabold tracking-tight mb-4 bg-gradient-to-r from-blue-500 to-cyan-400 text-transparent bg-clip-text relative z-10" itemProp="name">
          Ginboot
        </h1>
        <h2 className="text-2xl md:text-3xl font-semibold tracking-tight mb-6 text-fd-foreground relative z-10">
          The Best Go Web Framework for Modern APIs
        </h2>
      </header>
      
      <p className="text-xl md:text-2xl text-fd-muted-foreground max-w-3xl mb-12" itemProp="description">
        Ginboot is an enterprise-ready, high-performance Golang REST API framework built on top of Gin. Designed for maximum developer productivity with out-of-the-box MongoDB, SQL, and DynamoDB integration, AWS Lambda serverless execution, and OpenTelemetry support.
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

      <section className="mt-24 grid grid-cols-1 md:grid-cols-3 gap-8 max-w-5xl w-full text-left" aria-label="Key Features">
        <article className="p-6 border border-fd-border rounded-2xl bg-fd-card">
          <h3 className="text-xl font-bold mb-2 text-fd-foreground">Database Agnostic</h3>
          <p className="text-fd-muted-foreground">Built-in generic repositories for MongoDB, SQL, and DynamoDB allowing instant CRUD operations, drastically reducing boilerplate Go code.</p>
        </article>
        <article className="p-6 border border-fd-border rounded-2xl bg-fd-card">
          <h3 className="text-xl font-bold mb-2 text-fd-foreground">AWS Lambda Serverless</h3>
          <p className="text-fd-muted-foreground">Automatically detects AWS Lambda environments and seamlessly proxies API Gateway requests, making it the premier serverless Go framework.</p>
        </article>
        <article className="p-6 border border-fd-border rounded-2xl bg-fd-card">
          <h3 className="text-xl font-bold mb-2 text-fd-foreground">Enterprise Telemetry</h3>
          <p className="text-fd-muted-foreground">Built-in OpenTelemetry plugin to ship traces, metrics, and logs straight to Grafana, providing deep observability for large-scale microservices.</p>
        </article>
      </section>
    </main>
  );
}

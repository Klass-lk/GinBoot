'use client';

import { useEffect, useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { cn } from '@/lib/cn';

export function InstallCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 2000);

    return () => clearTimeout(timer);
  }, [copied]);

  return (
    <div className="glass-card flex w-full max-w-md items-center gap-3 px-4 py-2.5 font-mono text-sm">
      <span aria-hidden className="select-none text-brand">
        $
      </span>
      <code className="flex-1 overflow-x-auto whitespace-nowrap text-left text-fd-foreground">
        {command}
      </code>
      <button
        type="button"
        onClick={() => {
          void navigator.clipboard.writeText(command).then(() => setCopied(true));
        }}
        aria-label={copied ? 'Copied' : 'Copy install command'}
        className={cn(
          'shrink-0 rounded-md p-1.5 transition-colors',
          'text-fd-muted-foreground hover:bg-fd-accent hover:text-fd-foreground',
        )}
      >
        {copied ? <Check className="size-4 text-brand" /> : <Copy className="size-4" />}
      </button>
    </div>
  );
}

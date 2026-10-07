'use client';
import { Check, Copy } from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';

export function CopyButton({ text, label = 'Copy' }: { text: string; label?: string }) {
  const [done, setDone] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setDone(true);
      window.setTimeout(() => setDone(false), 2000);
    } catch {
      setDone(false);
    }
  }
  return (
    <Button variant="secondary" size="sm" onClick={() => void copy()}>
      {done ? <Check className="h-3.5 w-3.5" aria-hidden /> : <Copy className="h-3.5 w-3.5" aria-hidden />}
      {done ? 'Copied' : label}
    </Button>
  );
}

export function CodeBlock({ title, code }: { title: string; code: string }) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium">{title}</p>
        <CopyButton text={code} label={`Copy ${title}`} />
      </div>
      <pre tabIndex={0} className="overflow-x-auto rounded-md border bg-muted p-3 font-mono text-xs leading-relaxed">
        {code}
      </pre>
    </div>
  );
}

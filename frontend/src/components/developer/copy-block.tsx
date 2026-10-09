'use client';
import { Check, Copy } from 'lucide-react';
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { useTranslations } from '@/i18n/use-translations';

export function CopyButton({ text, label }: { text: string; label?: string }) {
  const t = useTranslations('common');
  const [done, setDone] = useState(false);
  const [failed, setFailed] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setFailed(false);
      setDone(true);
      window.setTimeout(() => setDone(false), 2000);
    } catch {
      setDone(false);
      setFailed(true);
      window.setTimeout(() => setFailed(false), 6000);
    }
  }
  return (
    <span className="inline-flex flex-wrap items-center justify-end gap-2">
      <Button variant="secondary" size="sm" onClick={() => void copy()}>
        {done ? <Check className="h-3.5 w-3.5" aria-hidden /> : <Copy className="h-3.5 w-3.5" aria-hidden />}
        {done ? t('copied') : (label ?? t('copy'))}
      </Button>
      {failed ? (
        <span role="alert" className="text-xs text-danger">
          {t('copyFailed')}
        </span>
      ) : null}
    </span>
  );
}

export function CodeBlock({ title, code }: { title: string; code: string }) {
  const t = useTranslations('common');
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium">{title}</p>
        <CopyButton text={code} label={t('copyTitle', { title })} />
      </div>
      <pre tabIndex={0} className="overflow-x-auto rounded-md border bg-muted p-3 font-mono text-xs leading-relaxed">
        {code}
      </pre>
    </div>
  );
}

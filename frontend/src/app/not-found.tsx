'use client';
import Link from 'next/link';
import { SearchX } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useTranslations } from '@/i18n/use-translations';

/**
 * Unknown URLs: a translated page with a next step, instead of Next's English default. Only `common` is needed, which
 * the root scope loads. Styled like EmptyState, but with a real <h1>, since this is the whole page.
 */
export default function NotFound() {
  const t = useTranslations('common.notFound');
  return (
    <main id="main" tabIndex={-1} className="mx-auto flex min-h-screen max-w-md items-center px-4 focus:outline-none">
      <div className="w-full rounded-lg border border-dashed px-6 py-12 text-center">
        <span aria-hidden className="mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <SearchX className="h-5 w-5" />
        </span>
        <h1 className="text-base font-semibold">{t('title')}</h1>
        <p className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">{t('body')}</p>
        <div className="mt-4">
          <Button asChild>
            <Link href="/dashboard">{t('action')}</Link>
          </Button>
        </div>
      </div>
    </main>
  );
}

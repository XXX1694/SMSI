'use client';
import Link from 'next/link';
import { useTranslations } from '@/i18n/use-translations';
import { cn } from '@/lib/utils';

/** "Terms · Privacy" links for footers. */
export function LegalLinks({ className }: { className?: string }) {
  const t = useTranslations('legal');
  return (
    <p className={cn('flex gap-3 text-xs text-muted-foreground', className)}>
      <Link href="/terms" className="inline-flex min-h-11 items-center hover:text-foreground hover:underline md:min-h-0">
        {t('terms')}
      </Link>
      <Link href="/privacy" className="inline-flex min-h-11 items-center hover:text-foreground hover:underline md:min-h-0">
        {t('privacy')}
      </Link>
    </p>
  );
}

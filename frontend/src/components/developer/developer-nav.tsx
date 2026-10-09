'use client';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { cn } from '@/lib/utils';
import { useTranslations } from '@/i18n/use-translations';

const ITEMS = [
  { href: '/developer', labelKey: 'navKeys' },
  { href: '/developer/mcp', labelKey: 'navMcp' },
] as const;

export function DeveloperNav() {
  const t = useTranslations('developer');
  const pathname = usePathname();
  return (
    <nav aria-label={t('navLabel')} className="mb-8 flex gap-1 border-b">
      {ITEMS.map((i) => {
        const active = pathname === i.href;
        return (
          <Link
            key={i.href}
            href={i.href}
            aria-current={active ? 'page' : undefined}
            className={cn('-mb-px border-b-2 px-3 py-2 text-sm font-medium', active ? 'border-accent text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground')}
          >
            {t(i.labelKey)}
          </Link>
        );
      })}
    </nav>
  );
}

'use client';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { cn } from '@/lib/utils';

const ITEMS = [
  { href: '/developer', label: 'API keys & logs' },
  { href: '/developer/mcp', label: 'MCP connections' },
];

export function DeveloperNav() {
  const pathname = usePathname();
  return (
    <nav aria-label="Developer sections" className="mb-8 flex gap-1 border-b">
      {ITEMS.map((i) => {
        const active = pathname === i.href;
        return (
          <Link
            key={i.href}
            href={i.href}
            aria-current={active ? 'page' : undefined}
            className={cn('-mb-px border-b-2 px-3 py-2 text-sm font-medium', active ? 'border-accent text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground')}
          >
            {i.label}
          </Link>
        );
      })}
    </nav>
  );
}

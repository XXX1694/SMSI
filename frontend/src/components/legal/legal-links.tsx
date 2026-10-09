import Link from 'next/link';
import { cn } from '@/lib/utils';

/** "Terms · Privacy" links for footers. */
export function LegalLinks({ className }: { className?: string }) {
  return (
    <p className={cn('flex gap-3 text-xs text-muted-foreground', className)}>
      <Link href="/terms" className="hover:text-foreground hover:underline">
        Terms
      </Link>
      <Link href="/privacy" className="hover:text-foreground hover:underline">
        Privacy
      </Link>
    </p>
  );
}

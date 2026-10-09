import { cn } from '@/lib/utils';

/** Placeholder block with a shimmer sweep (static under reduced motion). */
export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cn('shimmer rounded-md bg-muted', className)} />;
}

import * as React from 'react';
import { cn } from '@/lib/utils';

/** The bordered surface used for list items and panels. Render it as a `li` or `section` with `as`. */
export function Card({ as: Tag = 'div', className, ...props }: React.HTMLAttributes<HTMLElement> & { as?: 'div' | 'li' | 'section' | 'article' }) {
  return <Tag className={cn('rounded-lg border bg-background p-4', className)} {...props} />;
}

/** A titled group on a page, with an optional action next to the heading. */
export function Section({ title, action, children }: { title: string; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  );
}

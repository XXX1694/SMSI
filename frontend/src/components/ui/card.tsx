import * as React from 'react';
import { cn } from '@/lib/utils';

type CardProps = React.HTMLAttributes<HTMLElement> & {
  as?: 'div' | 'li' | 'section' | 'article';
  /** `glass` is for panels and tiles that are read at a glance; lists, forms and long text stay `solid` (D-024). */
  surface?: 'solid' | 'glass';
};

/** The bordered surface used for list items and panels. Render it as a `li` or `section` with `as`. */
export function Card({ as: Tag = 'div', surface = 'solid', className, ...props }: CardProps) {
  return <Tag className={cn('rounded-lg border p-4', surface === 'glass' ? 'glass-card rounded-xl' : 'bg-background', className)} {...props} />;
}

/** A titled group on a page, with an optional action next to the heading. */
export function Section({ title, action, children }: { title: React.ReactNode; action?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="min-w-0 text-sm font-semibold tracking-tight">{title}</h2>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
      {children}
    </section>
  );
}

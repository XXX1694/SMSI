import * as React from 'react';
import { cn } from '@/lib/utils';

/**
 * The one table of the app. On md and up it is a normal table inside a keyboard-focusable, labelled scroll region
 * (axe `scrollable-region-focusable`). Below md every row becomes a stacked card where each cell shows its
 * column title (`<Td label>`), so no column is ever hidden or scrolled out of sight. The ARIA roles are explicit
 * because `display: block` makes some browsers drop the table semantics.
 */
export function Table({ label, className, children }: { label: string; className?: string; children: React.ReactNode }) {
  return (
    <div
      role="region"
      aria-label={label}
      tabIndex={0}
      className={cn('overflow-x-auto rounded-lg border', className)}
    >
      <table role="table" className="w-full text-left text-sm max-md:block">
        {children}
      </table>
    </div>
  );
}

export function Thead({ className, ...props }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <thead role="rowgroup" className={cn('border-b bg-muted/50 text-xs text-muted-foreground max-md:sr-only', className)} {...props} />;
}

export function Tbody({ className, ...props }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody role="rowgroup" className={cn('divide-y max-md:block', className)} {...props} />;
}

export function Tr({ className, ...props }: React.HTMLAttributes<HTMLTableRowElement>) {
  return <tr role="row" className={cn('max-md:block max-md:px-3 max-md:py-2.5', className)} {...props} />;
}

type Align = 'left' | 'right';

export function Th({ align = 'left', className, ...props }: React.ThHTMLAttributes<HTMLTableCellElement> & { align?: Align }) {
  return <th role="columnheader" scope="col" className={cn('px-3 py-2 font-medium', align === 'right' && 'text-right', className)} {...props} />;
}

/** `label` is the column title shown next to the value in the stacked (mobile) layout; omit it for an actions cell. */
export function Td({
  label,
  align = 'left',
  className,
  ...props
}: React.TdHTMLAttributes<HTMLTableCellElement> & { label?: string; align?: Align }) {
  return (
    <td
      role="cell"
      data-label={label}
      className={cn(
        'px-3 py-2',
        align === 'right' && 'text-right',
        'max-md:flex max-md:items-baseline max-md:gap-3 max-md:px-0 max-md:py-0.5 max-md:text-left max-md:leading-snug',
        'max-md:data-[label]:before:w-20 max-md:data-[label]:before:shrink-0 max-md:data-[label]:before:[overflow-wrap:anywhere] max-md:data-[label]:before:text-xs max-md:data-[label]:before:text-muted-foreground max-md:data-[label]:before:content-[attr(data-label)]',
        className,
      )}
      {...props}
    />
  );
}

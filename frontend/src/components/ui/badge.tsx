import { cva, type VariantProps } from 'class-variance-authority';
import * as React from 'react';
import { StatusGlyph } from '@/components/ui/status-glyph';
import type { GlyphName } from '@/lib/status';
import { cn } from '@/lib/utils';

/**
 * A tag, not a pill (D-024, BRAND.md section 5): 5 px radius, a hairline border, no fill. With a `glyph` (statuses) only the
 * glyph carries the tone and the label stays in the text colour; without one (risk, trust, capability) the label takes the
 * tone. Every tone colour is AA on the background and on glass, so either way the text passes.
 */
const badgeVariants = cva(
  'inline-flex h-[1.375rem] items-center gap-1.5 whitespace-nowrap rounded-[5px] border px-1.5 text-xs font-medium leading-none tabular-nums',
  {
    variants: {
      tone: {
        neutral: 'border-border text-muted-foreground',
        outline: 'border-border text-muted-foreground',
        info: 'border-info/30 text-info',
        accent: 'border-accent/30 text-accent',
        success: 'border-success/30 text-success',
        warning: 'border-warning/35 text-warning',
        danger: 'border-danger/35 text-danger',
      },
      withGlyph: { true: '', false: '' },
    },
    compoundVariants: [
      { withGlyph: true, tone: ['info', 'accent', 'success', 'warning'], className: 'border-border text-foreground' },
      { withGlyph: true, tone: 'danger', className: 'text-foreground' },
    ],
    defaultVariants: { tone: 'neutral', withGlyph: false },
  },
);

const GLYPH_TONE: Record<NonNullable<BadgeProps['tone']>, string> = {
  neutral: 'text-muted-foreground',
  outline: 'text-muted-foreground',
  info: 'text-info',
  accent: 'text-accent',
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
};

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement>, Omit<VariantProps<typeof badgeVariants>, 'withGlyph'> {
  /** The status shape before the label; see StatusGlyph. */
  glyph?: GlyphName;
}

export function Badge({ className, tone, glyph, children, ...props }: BadgeProps) {
  return (
    <span className={cn(badgeVariants({ tone, withGlyph: Boolean(glyph) }), className)} {...props}>
      {glyph ? <StatusGlyph name={glyph} className={GLYPH_TONE[tone ?? 'neutral']} /> : null}
      {children}
    </span>
  );
}

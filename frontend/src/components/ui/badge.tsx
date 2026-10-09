import { cva, type VariantProps } from 'class-variance-authority';
import * as React from 'react';
import { StatusGlyph } from '@/components/ui/status-glyph';
import type { GlyphName } from '@/lib/status';
import { cn } from '@/lib/utils';

/**
 * A tag, not a pill (D-024, BRAND.md section 5): a small radius, a hairline border, no fill. Without a glyph (risk, trust,
 * capability) the label takes the tone. With a `glyph` (statuses) the glyph takes the tone and the label keeps the text
 * colour, muted for the quiet neutral statuses. Every tone colour is AA on the background and on glass.
 */
const badgeVariants = cva(
  'inline-flex h-tag items-center gap-1.5 whitespace-nowrap rounded-tag border px-1.5 text-xs font-medium leading-none tabular-nums',
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
    },
    defaultVariants: { tone: 'neutral' },
  },
);

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement>, VariantProps<typeof badgeVariants> {
  /** The status shape before the label; see StatusGlyph. */
  glyph?: GlyphName;
}

export function Badge({ className, tone, glyph, children, ...props }: BadgeProps) {
  // With a glyph the tag keeps its tone, the glyph takes it through currentColor, and the label goes back to the text
  // colour (quiet neutral statuses stay muted). The tinted border stays only on the failed tag, as a second cue.
  const loud = glyph && tone && tone !== 'neutral' && tone !== 'outline';
  return (
    <span className={cn(badgeVariants({ tone }), loud && tone !== 'danger' && 'border-border', className)} {...props}>
      {glyph ? <StatusGlyph name={glyph} /> : null}
      {loud ? <span className="text-foreground">{children}</span> : children}
    </span>
  );
}

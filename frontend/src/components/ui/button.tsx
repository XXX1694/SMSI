import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';
import * as React from 'react';
import { cn } from '@/lib/utils';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-[color,background-color,border-color,transform,opacity] ease-enter active:scale-[0.97] disabled:pointer-events-none disabled:opacity-50 motion-reduce:active:scale-100',
  {
    variants: {
      variant: {
        primary: 'bg-accent text-accent-foreground hover:bg-accent/90',
        secondary: 'border border-input bg-background hover:bg-muted',
        ghost: 'hover:bg-muted',
        danger: 'bg-danger text-background hover:bg-danger/90',
        link: 'text-accent underline-offset-4 hover:underline',
      },
      // Below md the minimum is 44 px (touch targets); from md up the compact sizes stay.
      size: { default: 'h-9 px-4 max-md:h-11', sm: 'h-8 px-3 text-compact max-md:h-11', icon: 'h-9 w-9 max-md:h-11 max-md:w-11' },
    },
    defaultVariants: { variant: 'primary', size: 'default' },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  /** Disables the button and tells assistive tech it is working. The caller still changes the label. */
  loading?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, loading = false, disabled, type, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button';
    return (
      <Comp
        ref={ref}
        type={asChild ? undefined : (type ?? 'button')}
        className={cn(buttonVariants({ variant, size }), className)}
        disabled={disabled || loading}
        aria-busy={loading || undefined}
        {...props}
      />
    );
  },
);
Button.displayName = 'Button';

export { buttonVariants };

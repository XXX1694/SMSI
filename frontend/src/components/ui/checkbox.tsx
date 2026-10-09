'use client';
import * as CheckboxPrimitive from '@radix-ui/react-checkbox';
import { Check } from 'lucide-react';
import * as React from 'react';
import { Hint, Label } from '@/components/ui/input';
import { cn } from '@/lib/utils';

export function Checkbox({ className, ...props }: React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      className={cn(
        'mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-sm border border-input bg-background data-[state=checked]:border-accent data-[state=checked]:bg-accent data-[state=checked]:text-accent-foreground disabled:opacity-50',
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator>
        <Check className="h-3 w-3" aria-hidden />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );
}

/** A checkbox with its label (and optional description) linked by id, so callers never pair them by hand. */
export function CheckboxField({
  label,
  description,
  id,
  ...props
}: Omit<React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>, 'children'> & { label: React.ReactNode; description?: React.ReactNode }) {
  const generated = React.useId();
  const checkboxId = id ?? generated;
  const descriptionId = description ? `${checkboxId}-description` : undefined;
  return (
    <div className="flex items-start gap-2.5">
      <Checkbox id={checkboxId} aria-describedby={descriptionId} {...props} />
      <div className="space-y-0.5">
        <Label htmlFor={checkboxId} className="font-normal">
          {label}
        </Label>
        {description ? <Hint id={descriptionId}>{description}</Hint> : null}
      </div>
    </div>
  );
}

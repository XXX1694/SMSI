'use client';
import * as React from 'react';
import { useTranslations } from '@/i18n/use-translations';
import { cn } from '@/lib/utils';

const field =
  'w-full rounded-md border border-input bg-background px-3 text-sm placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50 aria-[invalid=true]:border-danger';

interface FieldContextValue {
  id: string;
  describedBy: string | undefined;
  invalid: boolean;
  required: boolean;
}
const FieldContext = React.createContext<FieldContextValue | null>(null);

type ControlProps = {
  id?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: React.AriaAttributes['aria-invalid'];
  'aria-required'?: React.AriaAttributes['aria-required'];
};

/**
 * Inside a Field, a control gets its id, `aria-describedby`, `aria-invalid` and `aria-required` for free. Anything the
 * caller passes explicitly wins, so a control also works on its own.
 */
export function useFieldControl<T extends ControlProps>(own: T): T {
  const ctx = React.useContext(FieldContext);
  if (!ctx) return own;
  return {
    ...own,
    id: own.id ?? ctx.id,
    'aria-describedby': own['aria-describedby'] ?? ctx.describedBy,
    'aria-invalid': own['aria-invalid'] ?? (ctx.invalid ? true : undefined),
    'aria-required': own['aria-required'] ?? (ctx.required ? true : undefined),
  };
}

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => <input ref={ref} className={cn(field, 'h-9', className)} {...useFieldControl(props)} />,
);
Input.displayName = 'Input';

export const Textarea = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(
  ({ className, ...props }, ref) => <textarea ref={ref} className={cn(field, 'min-h-[96px] py-2 leading-relaxed', className)} {...useFieldControl(props)} />,
);
Textarea.displayName = 'Textarea';

export const Select = React.forwardRef<HTMLSelectElement, React.SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, ...props }, ref) => <select ref={ref} className={cn(field, 'h-9 pr-8', className)} {...useFieldControl(props)} />,
);
Select.displayName = 'Select';

export function Label({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  // eslint-disable-next-line jsx-a11y/label-has-associated-control
  return <label className={cn('block text-sm font-medium', className)} {...props} />;
}

export function Hint({ className, ...props }: React.HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cn('text-xs text-muted-foreground', className)} {...props} />;
}

export function FieldError({ className, ...props }: React.HTMLAttributes<HTMLParagraphElement>) {
  return <p role="alert" className={cn('text-xs text-danger', className)} {...props} />;
}

/**
 * Label, control, hint and error as one unit. The control child (Input, Textarea, Select, SecretInput) is wired up
 * through context. `htmlFor` is only for call sites that still pass their own control id.
 */
export function Field({
  label,
  hint,
  error,
  required,
  optional,
  htmlFor,
  children,
}: {
  label: string;
  hint?: string;
  error?: string | null;
  required?: boolean;
  /** Appends "(optional)" to the label. */
  optional?: boolean;
  htmlFor?: string;
  children: React.ReactNode;
}) {
  const t = useTranslations('common');
  const generated = React.useId();
  const id = htmlFor ?? generated;
  const showHint = Boolean(hint) && !error;
  const describedBy = error ? `${id}-error` : showHint ? `${id}-hint` : undefined;
  const value = React.useMemo(() => ({ id, describedBy, invalid: Boolean(error), required: Boolean(required) }), [id, describedBy, error, required]);
  return (
    <FieldContext.Provider value={value}>
      <div className="space-y-1.5">
        <Label htmlFor={id}>{optional ? t('optionalLabel', { label }) : label}</Label>
        {children}
        {showHint ? <Hint id={`${id}-hint`}>{hint}</Hint> : null}
        {error ? <FieldError id={`${id}-error`}>{error}</FieldError> : null}
      </div>
    </FieldContext.Provider>
  );
}

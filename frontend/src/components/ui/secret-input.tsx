'use client';
import { Eye, EyeOff } from 'lucide-react';
import * as React from 'react';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';

type Props = Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type' | 'autoComplete'> & {
  /** Accessible name for the toggle, e.g. "Webhook URL". */
  label: string;
};

/**
 * A password input with a show/hide toggle for credentials the user pastes. Password managers and
 * the browser are told not to store or suggest it, and it starts hidden every time it is mounted.
 */
export const SecretInput = React.forwardRef<HTMLInputElement, Props>(({ className, label, ...props }, ref) => {
  const [shown, setShown] = React.useState(false);
  return (
    <div className="relative">
      <Input
        ref={ref}
        {...props}
        type={shown ? 'text' : 'password'}
        autoComplete="new-password"
        autoCapitalize="off"
        autoCorrect="off"
        spellCheck={false}
        data-1p-ignore
        data-lpignore="true"
        className={cn('pr-10', className)}
      />
      <button
        type="button"
        onClick={() => setShown((s) => !s)}
        aria-pressed={shown}
        aria-label={`${shown ? 'Hide' : 'Show'} ${label}`}
        className="absolute inset-y-0 right-0 flex w-10 items-center justify-center rounded-r-md text-muted-foreground hover:text-foreground"
      >
        {shown ? <EyeOff className="h-4 w-4" aria-hidden /> : <Eye className="h-4 w-4" aria-hidden />}
      </button>
    </div>
  );
});
SecretInput.displayName = 'SecretInput';

'use client';
import { Eye, EyeOff } from 'lucide-react';
import * as React from 'react';
import { Input } from '@/components/ui/input';
import { useTranslations } from '@/i18n/use-translations';
import { cn } from '@/lib/utils';

type Props = Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type'> & {
  /** Accessible name of the toggle, e.g. "Password": it reads "Show Password" / "Hide Password" (common.showField). */
  label: string;
};

/**
 * The account password: a text field with a show/hide toggle that, unlike SecretInput, stays visible to password
 * managers (pass `autoComplete="current-password"` or `"new-password"`). It starts hidden every time it is mounted.
 */
export const PasswordInput = React.forwardRef<HTMLInputElement, Props>(({ className, label, ...props }, ref) => {
  const t = useTranslations('common');
  const [shown, setShown] = React.useState(false);
  return (
    <div className="relative">
      <Input
        ref={ref}
        {...props}
        type={shown ? 'text' : 'password'}
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        className={cn('pr-11', className)}
      />
      <button
        type="button"
        onClick={() => setShown((s) => !s)}
        aria-pressed={shown}
        aria-label={t(shown ? 'hideField' : 'showField', { label })}
        className="absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-r-md text-muted-foreground hover:text-foreground"
      >
        {shown ? <EyeOff className="h-4 w-4" aria-hidden /> : <Eye className="h-4 w-4" aria-hidden />}
      </button>
    </div>
  );
});
PasswordInput.displayName = 'PasswordInput';

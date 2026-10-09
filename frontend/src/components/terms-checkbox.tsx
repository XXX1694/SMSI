'use client';
import Link from 'next/link';
import { Checkbox } from '@/components/ui/checkbox';
import { nodes } from '@/i18n/rich';
import { useTranslations } from '@/i18n/use-translations';

/** "I agree to the Terms and the Privacy Policy" (D-016): an explicit box, with its error under it instead of a silent disabled button. */
export function TermsCheckbox({ checked, onCheckedChange, error }: { checked: boolean; onCheckedChange: (checked: boolean) => void; error?: string }) {
  const t = useTranslations('auth');
  return (
    <div>
      <div className="flex items-start gap-2 text-sm">
        <Checkbox
          id="accept-terms"
          aria-labelledby="accept-terms-text"
          checked={checked}
          onCheckedChange={(v) => onCheckedChange(v === true)}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? 'accept-terms-error' : undefined}
        />
        {/* Not a <label>: a click on the Terms or Privacy link inside one would also toggle the box. */}
        <span
          id="accept-terms-text"
          className="leading-snug"
          onClick={(e) => {
            if (!(e.target as HTMLElement).closest('a')) onCheckedChange(!checked);
          }}
        >
          {nodes(
            t.rich('agree', {
              terms: (c) => (
                <Link href="/terms" target="_blank" className="text-accent hover:underline">
                  {c}
                </Link>
              ),
              privacy: (c) => (
                <Link href="/privacy" target="_blank" className="text-accent hover:underline">
                  {c}
                </Link>
              ),
            }),
          )}
        </span>
      </div>
      {error ? (
        <p id="accept-terms-error" className="mt-1.5 text-xs text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}

import { Notice } from '@/components/states';
import type { ValidationIssue } from '@/lib/composer';

/** Validation problems and the last API error, shown above the action buttons. */
export function Feedback({ issues, apiError }: { issues: ValidationIssue[]; apiError: string | null }) {
  return (
    <>
      {issues.length > 0 ? (
        <div role="alert" className="rounded-md border border-danger/30 bg-danger-soft px-3 py-2 text-sm">
          <p className="font-medium text-danger">{issues.length === 1 ? 'Fix 1 problem first' : `Fix ${issues.length} problems first`}</p>
          <ul className="mt-1 list-disc space-y-0.5 pl-5 text-muted-foreground">
            {issues.map((i, idx) => (
              <li key={idx}>{i.message}</li>
            ))}
          </ul>
        </div>
      ) : null}
      {apiError ? <Notice tone="danger">{apiError}</Notice> : null}
    </>
  );
}

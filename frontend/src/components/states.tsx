import { AlertCircle, Inbox } from 'lucide-react';
import type { ComponentType, ReactNode } from 'react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { Skeleton } from '@/components/ui/skeleton';
import { errorMessage } from '@/hooks';

export function LoadingRows({ rows = 3 }: { rows?: number }) {
  return (
    <div role="status" aria-label="Loading" className="space-y-3 animate-fade-in">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-12 w-full" />
      ))}
    </div>
  );
}

export function ErrorState({ error, onRetry, title = 'Could not load this', showRef = true }: { error: unknown; onRetry?: () => void; title?: string; showRef?: boolean }) {
  return (
    <div role="alert" className="flex items-start gap-3 rounded-lg border border-danger/30 bg-danger-soft p-4 text-sm">
      <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-danger" aria-hidden />
      <div className="flex-1">
        <p className="font-medium text-danger">{title}</p>
        <p className="text-muted-foreground">{errorMessage(error, showRef)}</p>
      </div>
      {onRetry ? (
        <Button variant="secondary" size="sm" onClick={onRetry}>
          Retry
        </Button>
      ) : null}
    </div>
  );
}

/** A compact error line for forms, dialogs and "load more" failures. Optional retry button. */
export function InlineError({ children, onRetry, retryLabel = 'Try again', retryDisabled, className }: { children: ReactNode; onRetry?: () => void; retryLabel?: string; retryDisabled?: boolean; className?: string }) {
  return (
    <div role="alert" className={cn('flex flex-wrap items-center justify-between gap-3 text-sm text-danger', className)}>
      <span>{children}</span>
      {onRetry ? (
        <Button variant="secondary" size="sm" onClick={onRetry} disabled={retryDisabled}>
          {retryLabel}
        </Button>
      ) : null}
    </div>
  );
}

export function EmptyState({ title, children, action, icon: Icon = Inbox }: { title: string; children?: ReactNode; action?: ReactNode; icon?: ComponentType<{ className?: string }> }) {
  return (
    <div className="animate-fade-in rounded-lg border border-dashed px-6 py-12 text-center">
      <span aria-hidden className="mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <Icon className="h-5 w-5" />
      </span>
      <p className="text-sm font-medium">{title}</p>
      {children ? <p className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">{children}</p> : null}
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

export function Notice({ tone = 'warning', children }: { tone?: 'warning' | 'danger' | 'info'; children: ReactNode }) {
  const cls = {
    warning: 'border-warning/30 bg-warning-soft',
    danger: 'border-danger/30 bg-danger-soft',
    info: 'border-border bg-muted',
  }[tone];
  return (
    <div role={tone === 'info' ? 'note' : 'alert'} className={`rounded-md border px-3 py-2 text-sm ${cls}`}>
      {children}
    </div>
  );
}

export function PageHeader({ title, description, actions }: { title: string; description?: string; actions?: ReactNode }) {
  return (
    <div className="mb-8 flex flex-wrap items-start justify-between gap-4">
      <div className="min-w-0">
        <h1 className="text-balance text-2xl font-semibold tracking-tight [view-transition-name:page-title]">{title}</h1>
        {description ? <p className="mt-1.5 max-w-prose text-pretty text-sm text-muted-foreground">{description}</p> : null}
      </div>
      {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
    </div>
  );
}

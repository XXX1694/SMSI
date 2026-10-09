'use client';
import { Check, X } from 'lucide-react';
import Link from 'next/link';
import { useCallback, useEffect, useState } from 'react';
import { EmptyState } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { useAsync } from '@/hooks';
import { api } from '@/lib/api';
import { ONBOARDING_COMPLETE_KEY, ONBOARDING_DISMISSED_KEY, onboardingSteps, requiredDone, type OnboardingFacts } from '@/lib/onboarding';
import { cn } from '@/lib/utils';
import { readStorage, writeStorage } from '@/lib/storage';

/** Dismissed and complete are UI preferences, so they live in this browser only. null until storage is read: no flash. */
function usePrefsFlags() {
  const [flags, setFlags] = useState<{ dismissed: boolean; complete: boolean } | null>(null);
  useEffect(
    () => setFlags({ dismissed: readStorage(ONBOARDING_DISMISSED_KEY) === '1', complete: readStorage(ONBOARDING_COMPLETE_KEY) === '1' }),
    [],
  );
  const dismiss = useCallback(() => {
    writeStorage(ONBOARDING_DISMISSED_KEY, '1');
    setFlags((f) => (f ? { ...f, dismissed: true } : f));
  }, []);
  const markComplete = useCallback(() => writeStorage(ONBOARDING_COMPLETE_KEY, '1'), []);
  const reset = useCallback(() => {
    writeStorage(ONBOARDING_DISMISSED_KEY, '0');
    writeStorage(ONBOARDING_COMPLETE_KEY, '0');
    setFlags({ dismissed: false, complete: false });
  }, []);
  return { flags, dismiss, markComplete, reset };
}

/** Every screen has a next step: a dismissed checklist must not leave a brand-new account with nothing to do. */
function NoAccountYet() {
  return (
    <EmptyState
      title="Connect your first account"
      action={
        <Button asChild>
          <Link href="/accounts">Connect account</Link>
        </Button>
      }
    >
      Connect a network to start publishing.
    </EmptyState>
  );
}

/**
 * "Get started" checklist. Each step ticks off from real data: accounts (from the dashboard summary), posts,
 * API keys, MCP connections and approvals. If the extra data cannot be read the checklist stays hidden instead of guessing.
 */
export function OnboardingChecklist({ connectedAccounts }: { connectedAccounts: number }) {
  const { flags, dismiss, markComplete, reset } = usePrefsFlags();
  if (!flags) return null;
  if (flags.dismissed || flags.complete) {
    return (
      <>
        {connectedAccounts === 0 ? <NoAccountYet /> : null}
        <div className="text-right">
          <Button variant="ghost" size="sm" onClick={reset}>
            Show setup checklist
          </Button>
        </div>
      </>
    );
  }
  return <Checklist connectedAccounts={connectedAccounts} onDismiss={dismiss} onComplete={markComplete} />;
}

function Checklist({ connectedAccounts, onDismiss, onComplete }: { connectedAccounts: number; onDismiss: () => void; onComplete: () => void }) {
  const load = useCallback(async (): Promise<Omit<OnboardingFacts, 'connectedAccounts'>> => {
    const [posts, apiKeys, mcpConnections, approvals] = await Promise.all([
      api.posts.list({ limit: 1 }),
      api.developer.apiKeys(),
      api.developer.mcpConnections(),
      api.approvals.list('all', 1),
    ]);
    return { hasPost: posts.items.length > 0, apiKeys, mcpConnections, hasApproval: approvals.items.length > 0 };
  }, []);
  const { data, error } = useAsync(load);
  const steps = data ? onboardingSteps({ ...data, connectedAccounts }) : null;
  const complete = steps ? requiredDone(steps) : false;
  useEffect(() => {
    if (complete) onComplete();
  }, [complete, onComplete]);
  useEffect(() => {
    if (error) console.error('onboarding checklist: could not load its data', error);
  }, [error]);

  if (error && connectedAccounts === 0) return <NoAccountYet />;
  if (!steps) return null;
  const doneCount = steps.filter((s) => s.done).length;
  const nextId = steps.find((s) => !s.done && !s.optional)?.id;

  return (
    <Card as="section" aria-labelledby="onboarding-title" className="space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 id="onboarding-title" className="text-sm font-semibold tracking-tight">
            {complete ? 'You are set up' : 'Get started'}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {complete ? 'Every required step is done. You can hide this list.' : `${doneCount} of ${steps.length} steps done.`}
          </p>
        </div>
        <Button variant="ghost" size="icon" onClick={onDismiss} aria-label="Dismiss setup checklist">
          <X className="h-4 w-4" aria-hidden />
        </Button>
      </div>
      <ol className="stagger divide-y rounded-md border">
        {steps.map((s) => (
          <li key={s.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-3" data-done={s.done}>
            <span
              aria-hidden
              className={cn('flex h-5 w-5 shrink-0 items-center justify-center rounded-full border', s.done ? 'border-accent bg-accent text-accent-foreground' : 'border-input')}
            >
              {s.done ? <Check className="h-3 w-3" /> : null}
            </span>
            <div className="min-w-0 flex-1 basis-56">
              <p className="text-sm font-medium">
                {s.title}
                {s.done ? <span className="sr-only"> (done)</span> : null}
                {s.optional ? <span className="ml-2 text-xs font-normal text-muted-foreground">Optional</span> : null}
              </p>
              <p className="text-xs text-muted-foreground">{s.hint}</p>
            </div>
            {s.done ? null : (
              <Button asChild size="sm" variant={s.id === nextId ? 'primary' : 'secondary'}>
                <Link href={s.href}>{s.action}</Link>
              </Button>
            )}
          </li>
        ))}
      </ol>
    </Card>
  );
}

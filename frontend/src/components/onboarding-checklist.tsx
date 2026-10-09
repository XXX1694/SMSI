'use client';
import { Check, X } from 'lucide-react';
import Link from 'next/link';
import { useCallback, useEffect, useState } from 'react';
import { EmptyState } from '@/components/states';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { useAsync } from '@/hooks';
import { api } from '@/lib/api';
import { ONBOARDING_DISMISSED_KEY, onboardingSteps, requiredDone, type OnboardingFacts } from '@/lib/onboarding';
import { readStorage, writeStorage } from '@/lib/storage';

/** Dismissal is a UI preference, so it lives in this browser only. */
function useDismissed(): [boolean | null, () => void] {
  const [dismissed, setDismissed] = useState<boolean | null>(null); // null until storage is read: no flash
  useEffect(() => setDismissed(readStorage(ONBOARDING_DISMISSED_KEY) === '1'), []);
  const dismiss = useCallback(() => {
    writeStorage(ONBOARDING_DISMISSED_KEY, '1');
    setDismissed(true);
  }, []);
  return [dismissed, dismiss];
}

/** Every screen has a next step: a dismissed checklist must not leave a brand-new account with nothing to do. */
function NoAccountYet() {
  return (
    <EmptyState
      title="Connect your first account"
      action={
        <Button asChild>
          <Link href="/accounts">Go to accounts</Link>
        </Button>
      }
    >
      Connect LinkedIn, Telegram or the mock provider to start publishing.
    </EmptyState>
  );
}

/**
 * "Get started" checklist. Each step ticks off from real data: accounts (from the dashboard summary), posts,
 * API keys, MCP connections and approvals. If the extra data cannot be read the checklist stays hidden instead of guessing.
 */
export function OnboardingChecklist({ connectedAccounts }: { connectedAccounts: number }) {
  const [dismissed, dismiss] = useDismissed();
  if (dismissed === null) return null;
  if (dismissed) return connectedAccounts === 0 ? <NoAccountYet /> : null;
  return <Checklist connectedAccounts={connectedAccounts} onDismiss={dismiss} />;
}

function Checklist({ connectedAccounts, onDismiss }: { connectedAccounts: number; onDismiss: () => void }) {
  const load = useCallback(async (): Promise<Omit<OnboardingFacts, 'connectedAccounts'>> => {
    const [posts, apiKeys, mcpConnections, approvals] = await Promise.all([
      api.posts.list({ limit: 1 }),
      api.developer.apiKeys(),
      api.developer.mcpConnections(),
      api.approvals.list('all', 1),
    ]);
    return { hasPost: posts.items.length > 0, apiKeys, mcpConnections, hasApproval: approvals.items.length > 0 };
  }, []);
  const { data } = useAsync(load);

  if (!data) return null;
  const steps = onboardingSteps({ ...data, connectedAccounts });
  const complete = requiredDone(steps);
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
              className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full border ${s.done ? 'border-accent bg-accent text-accent-foreground' : 'border-input'}`}
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

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
import { useTranslations } from '@/i18n/use-translations';

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
  const t = useTranslations('dashboard.onboarding');
  const tc = useTranslations('common');
  return (
    <EmptyState
      title={t('noAccountTitle')}
      action={
        <Button asChild>
          <Link href="/accounts">{tc('connectAccount')}</Link>
        </Button>
      }
    >
      {t('noAccountBody')}
    </EmptyState>
  );
}

/**
 * "Get started" checklist. Each step ticks off from real data: accounts (from the dashboard summary), posts,
 * API keys, MCP connections and approvals. If the extra data cannot be read the checklist stays hidden instead of guessing.
 */
export function OnboardingChecklist({ connectedAccounts }: { connectedAccounts: number }) {
  const t = useTranslations('dashboard.onboarding');
  const { flags, dismiss, markComplete, reset } = usePrefsFlags();
  if (!flags) return null;
  if (flags.dismissed || flags.complete) {
    return (
      <>
        {connectedAccounts === 0 ? <NoAccountYet /> : null}
        <div className="text-right">
          <Button variant="ghost" size="sm" onClick={reset}>
            {t('showChecklist')}
          </Button>
        </div>
      </>
    );
  }
  return <Checklist connectedAccounts={connectedAccounts} onDismiss={dismiss} onComplete={markComplete} />;
}

function SetUpLine({ onExpand, onDismiss }: { onExpand: () => void; onDismiss: () => void }) {
  const t = useTranslations('dashboard.onboarding');
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-3 rounded-lg border px-4 py-2 text-sm">
      <p className="flex items-center gap-2 font-medium">
        <Check className="h-4 w-4 text-success" aria-hidden /> {t('setUp')}
      </p>
      <span className="flex items-center gap-1">
        <Button variant="ghost" size="sm" onClick={onExpand}>
          {t('showSteps')}
        </Button>
        <Button variant="ghost" size="icon" onClick={onDismiss} aria-label={t('dismiss')}>
          <X className="h-4 w-4" aria-hidden />
        </Button>
      </span>
    </div>
  );
}

function Checklist({ connectedAccounts, onDismiss, onComplete }: { connectedAccounts: number; onDismiss: () => void; onComplete: () => void }) {
  const t = useTranslations('dashboard.onboarding');
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
  const [expanded, setExpanded] = useState(false);
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
  // Everything required is done: one quiet line instead of the whole list. "Show steps" expands it.
  if (complete && !expanded) {
    return <SetUpLine onExpand={() => setExpanded(true)} onDismiss={onDismiss} />;
  }
  const doneCount = steps.filter((s) => s.done).length;
  const nextId = steps.find((s) => !s.done && !s.optional)?.id;

  return (
    <Card as="section" aria-labelledby="onboarding-title" className="space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 id="onboarding-title" className="text-sm font-semibold tracking-tight">
            {complete ? t('doneTitle') : t('getStarted')}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {complete ? t('doneBody') : t('progress', { done: doneCount, total: steps.length })}
          </p>
        </div>
        <Button variant="ghost" size="icon" onClick={onDismiss} aria-label={t('dismiss')}>
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
                {t(`steps.${s.id}.title`)}
                {s.done ? <span className="sr-only"> {t('doneSr')}</span> : null}
                {s.optional ? <span className="ml-2 text-xs font-normal text-muted-foreground">{t('optional')}</span> : null}
              </p>
              <p className="text-xs text-muted-foreground">{t(`steps.${s.id}.hint`)}</p>
            </div>
            {s.done ? null : (
              <Button asChild size="sm" variant={s.id === nextId ? 'primary' : 'secondary'}>
                <Link href={s.href}>{t(`steps.${s.id}.action`)}</Link>
              </Button>
            )}
          </li>
        ))}
      </ol>
    </Card>
  );
}

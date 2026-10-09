'use client';
import { heroWipe } from '@/lib/hero';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { validateComposer, type ComposerState, type ValidationIssue } from '@/lib/composer';
import { postHref } from '@/lib/demo/config';
import { joinList } from '@/lib/format';
import type { CreatePostInput, Provider, SocialAccount } from '@/lib/types';
import { useErrorText } from '@/hooks';
import { Feedback } from './issue-list';
import { useTranslations } from '@/i18n/use-translations';

type Action = 'draft' | 'schedule' | 'publish';

function buildInput(state: ComposerState, title: string, action: Action): CreatePostInput {
  const targets = state.accountIds
    .filter((id) => state.overrides[id]?.trim())
    .map((id) => ({ social_account_id: id, content: (state.overrides[id] ?? '').trim() }));
  return {
    title: title.trim() || undefined,
    content: state.content,
    social_account_ids: state.accountIds,
    media_ids: state.media.map((m) => m.id),
    targets: targets.length ? targets : undefined,
    ...(action === 'schedule' && state.scheduledAtUtc ? { scheduled_at: state.scheduledAtUtc, schedule: true } : {}),
  };
}

interface Props {
  onLeave: () => void;
  state: ComposerState;
  title: string;
  accounts: SocialAccount[];
  providers: Provider[];
  selected: SocialAccount[];
}

const DONE = { draft: 'draftSaved', schedule: 'postScheduled', publish: 'publishStarted' } as const satisfies Record<Action, string>;

export function CreateActions({ state, title, accounts, providers, selected, onLeave }: Props) {
  const t = useTranslations();
  const tc = useTranslations('composer');
  const errorText = useErrorText();
  const router = useRouter();
  const toast = useToast();
  const [issues, setIssues] = useState<ValidationIssue[]>([]);
  const [apiError, setApiError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmPublish, setConfirmPublish] = useState(false);

  async function run(action: Action) {
    setBusy(true);
    setApiError(null);
    try {
      const post = await api.posts.create(buildInput(state, title, action));
      if (action === 'publish') await Promise.all([api.posts.publish(post.id), heroWipe()]);
      toast.success(tc(DONE[action]));
      onLeave();
      router.push(postHref(post.id));
    } catch (e) {
      setApiError(errorText(e));
      setBusy(false);
      throw e;
    }
  }

  async function submit(action: Action) {
    const found = validateComposer(state, accounts, providers, t, { requireSchedule: action === 'schedule' });
    setIssues(found);
    if (found.length > 0) return;
    if (action === 'publish') {
      setConfirmPublish(true);
      return;
    }
    try {
      await run(action);
    } catch {
      /* surfaced via apiError */
    }
  }

  return (
    <>
      <Feedback issues={issues} apiError={apiError} />
      <div className="flex flex-wrap gap-2 border-t pt-6">
        <Button variant="secondary" disabled={busy} onClick={() => void submit('draft')}>
          {tc('saveDraft')}
        </Button>
        <Button variant="secondary" disabled={busy} onClick={() => void submit('schedule')}>
          {tc('scheduleAction')}
        </Button>
        <Button disabled={busy} onClick={() => void submit('publish')}>
          {tc('publishNow')}
        </Button>
      </div>
      <ConfirmDialog
        open={confirmPublish}
        onOpenChange={setConfirmPublish}
        title={tc('publishConfirmTitle')}
        description={tc('publishConfirmBody', { accounts: joinList(selected.map((a) => a.display_name || a.username), t) })}
        confirmLabel={tc('publishNow')}
        onConfirm={() => run('publish')}
      />
    </>
  );
}

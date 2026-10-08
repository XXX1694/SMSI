'use client';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useCallback, useMemo, useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { usePrefs } from '@/components/prefs-provider';
import { EmptyState, ErrorState, LoadingRows, Notice } from '@/components/states';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { api } from '@/lib/api';
import { validateComposer, type ComposerState, type ValidationIssue } from '@/lib/composer';
import { postHref } from '@/lib/demo/config';
import { zonedToUtcIso } from '@/lib/time';
import type { CreatePostInput } from '@/lib/types';
import { errorMessage, useAsync } from '@/hooks';
import { AccountChips } from './account-chips';
import { ContentEditor } from './content-editor';
import { MediaSection } from './media-section';
import { PreviewsPanel } from './previews-panel';
import { ScheduleFields } from './schedule-fields';

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

export function ComposerView() {
  const router = useRouter();
  const toast = useToast();
  const { timezone } = usePrefs();
  const load = useCallback(async () => {
    const [providers, accounts] = await Promise.all([api.social.providers(), api.social.accounts()]);
    return { providers, accounts };
  }, []);
  const { data, error, loading, reload } = useAsync(load);

  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');
  const [overrides, setOverrides] = useState<Record<string, string>>({});
  const [accountIds, setAccountIds] = useState<string[]>([]);
  const [media, setMedia] = useState<ComposerState['media']>([]);
  const [date, setDate] = useState('');
  const [time, setTime] = useState('09:00');
  const [issues, setIssues] = useState<ValidationIssue[]>([]);
  const [apiError, setApiError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmPublish, setConfirmPublish] = useState(false);

  const scheduledAtUtc = useMemo(() => (date ? zonedToUtcIso(date, time, timezone) : null), [date, time, timezone]);
  const state: ComposerState = { content, overrides, accountIds, media, scheduledAtUtc };

  if (loading && !data) return <LoadingRows rows={4} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} />;
  const { providers, accounts } = data;
  const selected = accounts.filter((a) => accountIds.includes(a.id));

  if (accounts.length === 0) {
    return (
      <EmptyState
        title="No accounts connected"
        action={
          <Button asChild>
            <Link href="/accounts">Connect an account</Link>
          </Button>
        }
      >
        Connect at least one account before composing a post.
      </EmptyState>
    );
  }

  function check(action: Action): boolean {
    const found = validateComposer(state, accounts, providers, { requireSchedule: action === 'schedule' });
    setIssues(found);
    return found.length === 0;
  }

  async function run(action: Action) {
    setBusy(true);
    setApiError(null);
    try {
      const post = await api.posts.create(buildInput(state, title, action));
      if (action === 'publish') await api.posts.publish(post.id);
      toast.success(action === 'draft' ? 'Draft saved' : action === 'schedule' ? 'Post scheduled' : 'Publishing started');
      router.push(postHref(post.id));
    } catch (e) {
      setApiError(errorMessage(e));
      setBusy(false);
      throw e;
    }
  }

  async function submit(action: Action) {
    if (!check(action)) return;
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
    <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <div className="space-y-6">
        <Field label="Title (optional)" htmlFor="post-title" hint="For your own reference; not published.">
          <Input id="post-title" value={title} onChange={(e) => setTitle(e.target.value)} maxLength={120} />
        </Field>
        <div className="space-y-2">
          <h2 className="text-sm font-semibold">Publish to</h2>
          <AccountChips
            accounts={accounts}
            selected={accountIds}
            onToggle={(id) => setAccountIds((cur) => (cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]))}
          />
        </div>
        <ContentEditor
          content={content}
          overrides={overrides}
          selected={selected}
          providers={providers}
          onContent={setContent}
          onOverride={(id, v) => setOverrides((cur) => ({ ...cur, [id]: v }))}
        />
        <MediaSection media={media} onChange={setMedia} />
        <ScheduleFields date={date} time={time} timezone={timezone} utcIso={scheduledAtUtc} onDate={setDate} onTime={setTime} />
        {issues.length > 0 ? (
          <div role="alert" className="rounded-md border border-danger/30 bg-danger-soft px-3 py-2 text-sm">
            <p className="font-medium text-danger">Fix before continuing</p>
            <ul className="mt-1 list-disc space-y-0.5 pl-5 text-muted-foreground">
              {issues.map((i, idx) => (
                <li key={idx}>{i.message}</li>
              ))}
            </ul>
          </div>
        ) : null}
        {apiError ? <Notice tone="danger">{apiError}</Notice> : null}
        <div className="flex flex-wrap gap-2 border-t pt-6">
          <Button variant="secondary" disabled={busy} onClick={() => void submit('draft')}>
            Save draft
          </Button>
          <Button variant="secondary" disabled={busy} onClick={() => void submit('schedule')}>
            Schedule
          </Button>
          <Button disabled={busy} onClick={() => void submit('publish')}>
            Publish now
          </Button>
        </div>
      </div>
      <PreviewsPanel selected={selected} content={content} overrides={overrides} media={media} />
      <ConfirmDialog
        open={confirmPublish}
        onOpenChange={setConfirmPublish}
        title="Publish now?"
        description={`This posts immediately to ${selected.map((a) => a.display_name || a.username).join(', ')} and cannot be undone from SocialOS.`}
        confirmLabel="Publish now"
        onConfirm={() => run('publish')}
      />
    </div>
  );
}

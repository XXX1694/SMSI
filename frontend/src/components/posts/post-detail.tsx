'use client';
import { ExternalLink } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useCallback, useEffect, useState } from 'react';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { usePrefs } from '@/components/prefs-provider';
import { ErrorState, InlineError, LoadingRows, Notice, PageHeader } from '@/components/states';
import { Section } from '@/components/ui/card';
import { AttemptStatusBadge, PostStatusBadge, TargetStatusBadge } from '@/components/status-badge';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter } from '@/components/ui/dialog';
import { Field, Input } from '@/components/ui/input';
import { Table, Tbody, Td, Th, Thead, Tr } from '@/components/ui/table';
import { api } from '@/lib/api';
import { describeErrorCode, friendlyMessage, isTechnicalMessage } from '@/lib/errors';
import { editHref } from '@/lib/demo/config';
import { postLabel } from '@/lib/format';
import { providerLabel } from '@/lib/normalize';
import { editBlockedReason, postActions } from '@/lib/status';
import { formatDateTime, zonedToUtcIso } from '@/lib/time';
import type { Post, PublicationAttempt } from '@/lib/types';
import { errorMessage, useAsync } from '@/hooks';

function Targets({ post }: { post: Post }) {
  const { timezone } = usePrefs();
  return (
    <ul className="divide-y rounded-lg border">
      {post.targets.map((t) => (
        <li key={t.id} className="space-y-2 p-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm font-medium">{providerLabel(t.platform)}</p>
            <TargetStatusBadge status={t.status} />
          </div>
          <p className="whitespace-pre-wrap break-words text-sm text-muted-foreground">{t.content}</p>
          {t.error_message ? (
            <Notice tone="danger">
              <span className="font-medium">{describeErrorCode(t.error_code)}</span>
              {isTechnicalMessage(t.error_message) ? null : <> {t.error_message}</>}
            </Notice>
          ) : null}
          {t.status === 'needs_review' ? (
            <Notice>The outcome is unknown. Check the platform before retrying to avoid a duplicate.</Notice>
          ) : null}
          <p className="text-xs text-muted-foreground">
            {t.published_at ? `Published ${formatDateTime(t.published_at, timezone)} · ` : ''}
            {t.attempt_count} attempt{t.attempt_count === 1 ? '' : 's'}
            {t.external_url ? (
              <>
                {' · '}
                <a href={t.external_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-accent hover:underline">
                  View on platform <ExternalLink className="h-3 w-3" aria-hidden />
                </a>
              </>
            ) : null}
          </p>
        </li>
      ))}
    </ul>
  );
}

function Attempts({ attempts, post }: { attempts: PublicationAttempt[]; post: Post }) {
  const { timezone } = usePrefs();
  const platformOf = (id: string) => providerLabel(post.targets.find((t) => t.id === id)?.platform ?? '');
  if (attempts.length === 0) return <p className="text-sm text-muted-foreground">No publication attempts yet.</p>;
  return (
    <Table label="Publication attempts">
      <Thead>
        <Tr>
          <Th>Target</Th>
          <Th>#</Th>
          <Th>Started</Th>
          <Th>Result</Th>
          <Th>Error</Th>
        </Tr>
      </Thead>
      <Tbody>
        {attempts.map((a) => (
          <Tr key={a.id}>
            <Td label="Target">{platformOf(a.post_target_id)}</Td>
            <Td label="Attempt" className="tabular-nums">{a.attempt_no}</Td>
            <Td label="Started" className="whitespace-nowrap">{formatDateTime(a.started_at, timezone)}</Td>
            <Td label="Result">
              <AttemptStatusBadge status={a.status} />
            </Td>
            <Td label="Error" className="text-muted-foreground max-md:text-foreground">{a.error_message ? friendlyMessage(null, a.error_message) : '—'}</Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  );
}

function ScheduleDialog({ open, onOpenChange, onSubmit }: { open: boolean; onOpenChange: (o: boolean) => void; onSubmit: (iso: string) => Promise<void> }) {
  const { timezone } = usePrefs();
  const [date, setDate] = useState('');
  const [time, setTime] = useState('09:00');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function go() {
    const iso = date ? zonedToUtcIso(date, time, timezone) : null;
    if (!iso || new Date(iso).getTime() < Date.now() + 60_000) {
      setError('Pick a date and time at least a minute in the future.');
      return;
    }
    setBusy(true);
    try {
      await onSubmit(iso);
      onOpenChange(false);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title="Schedule post" description={`Times are in ${timezone}.`}>
        <div className="flex gap-3">
          <Field label="Date" htmlFor="s-date">
            <Input id="s-date" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          </Field>
          <Field label="Time" htmlFor="s-time">
            <Input id="s-time" type="time" value={time} onChange={(e) => setTime(e.target.value)} />
          </Field>
        </div>
        {error ? <InlineError className="mt-3">{error}</InlineError> : null}
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={() => void go()} disabled={busy}>{busy ? 'Scheduling…' : 'Schedule'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type Dlg = 'publish' | 'cancel' | 'delete' | 'retry' | 'schedule' | null;

export function PostDetail({ id }: { id: string }) {
  const router = useRouter();
  const toast = useToast();
  const { timezone } = usePrefs();
  const load = useCallback(() => api.posts.get(id), [id]);
  const { data: post, error, loading, reload } = useAsync(load);
  const [dlg, setDlg] = useState<Dlg>(null);

  // Poll while the post is in flight.
  const inFlight = post?.status === 'publishing';
  useEffect(() => {
    if (!inFlight) return;
    const t = window.setInterval(reload, 2000);
    return () => window.clearInterval(t);
  }, [inFlight, reload]);

  if (loading && !post) return <LoadingRows rows={4} />;
  if (error || !post) return <ErrorState title="Could not load this post" showRef={false} error={error} onRetry={reload} />;
  const can = postActions(post.status);

  const act = (fn: () => Promise<unknown>, msg: string) => async () => {
    await fn();
    toast.success(msg);
    reload();
  };

  return (
    <>
      <PageHeader
        title={postLabel(post)}
        description={
          post.scheduled_at && post.status === 'scheduled'
            ? `Scheduled for ${formatDateTime(post.scheduled_at, timezone)}`
            : `Created ${formatDateTime(post.created_at, timezone)}${post.created_by === 'api_key' ? ' via API key' : ''}`
        }
        actions={
          <>
            <PostStatusBadge status={post.status} />
            {can.edit ? (
              <Button size="sm" variant="secondary" asChild>
                <Link href={editHref(post.id)}>Edit</Link>
              </Button>
            ) : (
              <Button size="sm" variant="secondary" disabled aria-describedby="edit-blocked">
                Edit
              </Button>
            )}
            {can.publish ? <Button size="sm" onClick={() => setDlg('publish')}>Publish now</Button> : null}
            {can.schedule ? <Button size="sm" variant="secondary" onClick={() => setDlg('schedule')}>Schedule</Button> : null}
            {can.retry ? <Button size="sm" variant="secondary" onClick={() => setDlg('retry')}>Retry failed</Button> : null}
            {can.cancel ? <Button size="sm" variant="secondary" onClick={() => setDlg('cancel')}>Cancel</Button> : null}
            {can.del ? <Button size="sm" variant="ghost" onClick={() => setDlg('delete')}>Delete</Button> : null}
          </>
        }
      />
      {can.edit ? null : (
        <p id="edit-blocked" className="-mt-4 mb-6 text-sm text-muted-foreground">
          {editBlockedReason(post.status)}
        </p>
      )}
      <div className="space-y-10">
        <Section title="Targets">
          <Targets post={post} />
        </Section>
        {post.media && post.media.length > 0 ? (
          <Section title="Media">
            <ul className="flex flex-wrap gap-2">
              {post.media.map((m) => (
                <li key={m.id} className="rounded-md border bg-muted px-2 py-1 text-xs text-muted-foreground">
                  {m.original_name}
                </li>
              ))}
            </ul>
          </Section>
        ) : null}
        <Section title="Attempt history">
          <Attempts attempts={post.attempts ?? []} post={post} />
        </Section>
      </div>
      <ConfirmDialog open={dlg === 'publish'} onOpenChange={(o) => !o && setDlg(null)} title="Publish now?" description="This posts immediately to every target." confirmLabel="Publish now" onConfirm={act(() => api.posts.publish(id), 'Publishing started')} />
      <ConfirmDialog open={dlg === 'retry'} onOpenChange={(o) => !o && setDlg(null)} title="Retry failed targets?" description="Only targets that failed are retried. Targets already published are left alone." confirmLabel="Retry" onConfirm={act(() => api.posts.retry(id), 'Retry started')} />
      <ConfirmDialog open={dlg === 'cancel'} onOpenChange={(o) => !o && setDlg(null)} title="Cancel this post?" description="It will not be published. Cancelled posts cannot be revived." confirmLabel="Cancel post" destructive onConfirm={act(() => api.posts.cancel(id), 'Post cancelled')} />
      <ConfirmDialog
        open={dlg === 'delete'}
        onOpenChange={(o) => !o && setDlg(null)}
        title="Delete this post?"
        description="It is removed from Steerpost. Content already published on a platform stays there."
        confirmLabel="Delete"
        destructive
        onConfirm={async () => {
          await api.posts.remove(id);
          toast.success('Post deleted');
          router.replace('/posts');
        }}
      />
      <ScheduleDialog open={dlg === 'schedule'} onOpenChange={(o) => !o && setDlg(null)} onSubmit={async (iso) => {
          await api.posts.schedule(id, iso);
          toast.success('Scheduled');
          reload();
        }}
      />
    </>
  );
}

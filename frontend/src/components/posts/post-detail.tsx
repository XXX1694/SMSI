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
import { describeErrorCode, friendlyMessage, isEnglish, isTechnicalMessage } from '@/lib/errors';
import { editHref } from '@/lib/demo/config';
import { joinList, postLabel } from '@/lib/format';
import { RetryPostDialog } from '@/components/posts/retry-post-dialog';
import { useProviderName } from '@/i18n/use-provider-name';
import { editBlockedReason, postActions } from '@/lib/status';
import { zonedToUtcIso } from '@/lib/time';
import type { Post, PublicationAttempt } from '@/lib/types';
import { useAsync, useErrorText } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';
import { useFormat } from '@/i18n/use-format';

function Targets({ post }: { post: Post }) {
  const t = useTranslations();
  const providerName = useProviderName();
  const fmt = useFormat();
  return (
    <ul className="divide-y rounded-lg border">
      {post.targets.map((tg) => (
        <li key={tg.id} className="space-y-2 p-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm font-medium">{providerName(tg.platform)}</p>
            <TargetStatusBadge status={tg.status} />
          </div>
          <p className="whitespace-pre-wrap break-words text-sm text-muted-foreground">{tg.content}</p>
          {tg.error_message ? (
            <Notice tone="danger">
              <span className="font-medium">{describeErrorCode(tg.error_code, t)}</span>
              {isTechnicalMessage(tg.error_message) || !isEnglish(t) ? null : <> {tg.error_message}</>}
            </Notice>
          ) : null}
          {tg.status === 'needs_review' ? <Notice>{t('posts.unconfirmedNote', { network: providerName(tg.platform) })}</Notice> : null}
          <p className="text-xs text-muted-foreground">
            {t('posts.targetMeta', { hasDate: String(Boolean(tg.published_at)), when: tg.published_at ? fmt.dateTime(tg.published_at) : '', count: tg.attempt_count })}
            {tg.external_url ? (
              <>
                {' · '}
                <a href={tg.external_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-accent hover:underline">
                  {t('posts.viewOn', { network: providerName(tg.platform) })} <ExternalLink className="h-3 w-3" aria-hidden />
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
  const t = useTranslations();
  const providerName = useProviderName();
  const tp = useTranslations('posts');
  const fmt = useFormat();
  const platformOf = (id: string) => providerName(post.targets.find((x) => x.id === id)?.platform ?? '');
  if (attempts.length === 0) return <p className="text-sm text-muted-foreground">{tp('noAttempts')}</p>;
  return (
    <Table label={tp('attemptsTable')}>
      <Thead>
        <Tr>
          <Th>{tp('colAccount')}</Th>
          <Th>{tp('colNumber')}</Th>
          <Th>{tp('colStarted')}</Th>
          <Th>{tp('colResult')}</Th>
          <Th>{tp('colError')}</Th>
        </Tr>
      </Thead>
      <Tbody>
        {attempts.map((a) => (
          <Tr key={a.id}>
            <Td label={tp('colAccount')}>{platformOf(a.post_target_id)}</Td>
            <Td label={tp('colAttempt')} className="tabular-nums">{a.attempt_no}</Td>
            <Td label={tp('colStarted')} className="whitespace-nowrap">{fmt.dateTime(a.started_at)}</Td>
            <Td label={tp('colResult')}>
              <AttemptStatusBadge status={a.status} />
            </Td>
            <Td label={tp('colError')} className="text-muted-foreground max-md:text-foreground">{a.error_message ? friendlyMessage(a.error_code, a.error_message, t) : '—'}</Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  );
}

function ScheduleDialog({ open, onOpenChange, onSubmit }: { open: boolean; onOpenChange: (o: boolean) => void; onSubmit: (iso: string) => Promise<void> }) {
  const t = useTranslations();
  const { timezone } = usePrefs();
  const errorText = useErrorText();
  const [date, setDate] = useState('');
  const [time, setTime] = useState('09:00');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function go() {
    const iso = date ? zonedToUtcIso(date, time, timezone) : null;
    if (!iso || new Date(iso).getTime() < Date.now() + 60_000) {
      setError(t('composer.v.tooSoon'));
      return;
    }
    setBusy(true);
    try {
      await onSubmit(iso);
      onOpenChange(false);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title={t('posts.scheduleTitle')} description={t('posts.scheduleBody', { timezone })}>
        <div className="flex gap-3">
          <Field label={t('composer.date')} htmlFor="s-date">
            <Input id="s-date" type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          </Field>
          <Field label={t('composer.time')} htmlFor="s-time">
            <Input id="s-time" type="time" value={time} onChange={(e) => setTime(e.target.value)} />
          </Field>
        </div>
        {error ? <InlineError className="mt-3">{error}</InlineError> : null}
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button onClick={() => void go()} disabled={busy}>{busy ? t('posts.scheduling') : t('composer.scheduleAction')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type Dlg = 'publish' | 'cancel' | 'delete' | 'retry' | 'schedule' | null;

export function PostDetail({ id }: { id: string }) {
  const t = useTranslations();
  const providerName = useProviderName();
  const fmt = useFormat();
  const router = useRouter();
  const toast = useToast();
  const load = useCallback(() => api.posts.get(id), [id]);
  const { data: post, error, loading, reload } = useAsync(load);
  const [dlg, setDlg] = useState<Dlg>(null);

  // Poll while the post is in flight.
  const inFlight = post?.status === 'publishing';
  useEffect(() => {
    if (!inFlight) return;
    const timer = window.setInterval(reload, 2000);
    return () => window.clearInterval(timer);
  }, [inFlight, reload]);

  if (loading && !post) return <LoadingRows rows={4} />;
  if (error || !post) return <ErrorState title={t('composer.loadFailed')} showRef={false} error={error} onRetry={reload} />;
  const can = postActions(post.status);

  const act = (fn: () => Promise<unknown>, msg: string) => async () => {
    await fn();
    toast.success(msg);
    reload();
  };

  return (
    <>
      <PageHeader
        title={postLabel(post, t)}
        description={
          post.scheduled_at && post.status === 'scheduled'
            ? t('posts.scheduledFor', { when: fmt.dateTime(post.scheduled_at) })
            : t(post.created_by === 'api_key' ? 'posts.createdAtByKey' : 'posts.createdAt', { when: fmt.dateTime(post.created_at) })
        }
        actions={
          <>
            <PostStatusBadge status={post.status} />
            {can.edit ? (
              <Button size="sm" variant="secondary" asChild>
                <Link href={editHref(post.id)}>{t('common.edit')}</Link>
              </Button>
            ) : (
              <Button size="sm" variant="secondary" disabled aria-describedby="edit-blocked">
                {t('common.edit')}
              </Button>
            )}
            {can.publish ? <Button size="sm" onClick={() => setDlg('publish')}>{t('composer.publishNow')}</Button> : null}
            {can.schedule ? <Button size="sm" variant="secondary" onClick={() => setDlg('schedule')}>{t('composer.scheduleAction')}</Button> : null}
            {can.retry ? <Button size="sm" variant="secondary" onClick={() => setDlg('retry')}>{t('common.retry')}</Button> : null}
            {can.cancel ? <Button size="sm" variant="secondary" onClick={() => setDlg('cancel')}>{t('posts.cancelPost')}</Button> : null}
            {can.del ? <Button size="sm" variant="ghost" onClick={() => setDlg('delete')}>{t('common.delete')}</Button> : null}
          </>
        }
      />
      {can.edit ? null : (
        <p id="edit-blocked" className="-mt-4 mb-6 text-sm text-muted-foreground">
          {editBlockedReason(post.status, t)}
        </p>
      )}
      <div className="space-y-10">
        <Section title={t('posts.sectionAccounts')}>
          <Targets post={post} />
        </Section>
        {post.media && post.media.length > 0 ? (
          <Section title={t('composer.media')}>
            <ul className="flex flex-wrap gap-2">
              {post.media.map((m) => (
                <li key={m.id} className="rounded-md border bg-muted px-2 py-1 text-xs text-muted-foreground">
                  {m.original_name}
                </li>
              ))}
            </ul>
          </Section>
        ) : null}
        <Section title={t('posts.sectionHistory')}>
          <Attempts attempts={post.attempts ?? []} post={post} />
        </Section>
      </div>
      <ConfirmDialog open={dlg === 'publish'} onOpenChange={(o) => !o && setDlg(null)} title={t('composer.publishConfirmTitle')} description={t('composer.publishConfirmBody', { accounts: joinList([...new Set(post.targets.map((x) => providerName(x.platform)))], t) })} confirmLabel={t('composer.publishNow')} onConfirm={act(() => api.posts.publish(id), t('composer.publishStarted'))} />
      <RetryPostDialog postId={id} open={dlg === 'retry'} onOpenChange={(o) => !o && setDlg(null)} onRetried={() => { toast.success(t('posts.retryStarted')); reload(); }} />
      <ConfirmDialog open={dlg === 'cancel'} onOpenChange={(o) => !o && setDlg(null)} title={t('posts.cancelTitle')} description={t('posts.cancelBody')} confirmLabel={t('posts.cancelPost')} dismissLabel={t('posts.keepPost')} destructive onConfirm={act(() => api.posts.cancel(id), t('posts.canceled'))} />
      <ConfirmDialog
        open={dlg === 'delete'}
        onOpenChange={(o) => !o && setDlg(null)}
        title={t('posts.deleteTitle')}
        description={t('posts.deleteBody')}
        confirmLabel={t('common.delete')}
        destructive
        onConfirm={async () => {
          await api.posts.remove(id);
          toast.success(t('posts.deleted'));
          router.replace('/posts');
        }}
      />
      <ScheduleDialog open={dlg === 'schedule'} onOpenChange={(o) => !o && setDlg(null)} onSubmit={async (iso) => {
          await api.posts.schedule(id, iso);
          toast.success(t('composer.postScheduled'));
          reload();
        }}
      />
    </>
  );
}

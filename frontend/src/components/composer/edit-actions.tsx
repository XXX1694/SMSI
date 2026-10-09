'use client';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { Notice } from '@/components/states';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { api } from '@/lib/api';
import { validateComposer, type ValidationIssue } from '@/lib/composer';
import { postHref } from '@/lib/demo/config';
import { buildUpdate, formFromPost, freshness, type FormValues } from '@/lib/post-edit';
import { postActions } from '@/lib/status';
import type { Post, Provider, SocialAccount } from '@/lib/types';
import { errorMessage } from '@/hooks';
import { Feedback } from './issue-list';
import type { ComposerFields } from './use-composer-fields';

export interface Baseline {
  post: Post;
  values: FormValues;
}

interface Props {
  fields: ComposerFields;
  baseline: Baseline;
  onBaseline: (b: Baseline) => void;
  onLeave: () => void;
  accounts: SocialAccount[];
  providers: Provider[];
}

type Kind = 'save' | 'schedule';

function Conflict({ latest, onLoad, onForce }: { latest: Post; onLoad: () => void; onForce: () => void }) {
  const stillEditable = postActions(latest.status).edit;
  return (
    <div className="space-y-3" role="alert">
      <Notice tone="danger">
        <span className="font-medium">This post changed since you opened it.</span>{' '}
        {stillEditable
          ? 'Someone or something (another tab, or an agent) saved a newer version. Saving now would overwrite it.'
          : `It is now ${latest.status.replace('_', ' ')} and can no longer be edited.`}
      </Notice>
      <div className="flex flex-wrap gap-2">
        <Button variant="secondary" size="sm" onClick={onLoad}>
          Load the latest version
        </Button>
        {stillEditable ? (
          <Button variant="ghost" size="sm" onClick={onForce}>
            Save mine anyway
          </Button>
        ) : null}
      </div>
    </div>
  );
}

export function EditActions({ fields, baseline, onBaseline, onLeave, accounts, providers }: Props) {
  const router = useRouter();
  const toast = useToast();
  const { post } = baseline;
  const [issues, setIssues] = useState<ValidationIssue[]>([]);
  const [apiError, setApiError] = useState<string | null>(null);
  const [conflict, setConflict] = useState<Post | null>(null);
  const [busy, setBusy] = useState(false);
  const { form, state } = fields;

  async function save(kind: Kind, force: boolean) {
    const timeChanged = form.date !== baseline.values.date || form.time !== baseline.values.time;
    const needTime = kind === 'schedule' || (post.status === 'scheduled' && (timeChanged || !form.date));
    const found = validateComposer(state, accounts, providers, { requireSchedule: needTime });
    setIssues(found);
    setApiError(null);
    if (found.length > 0) return;
    setBusy(true);
    try {
      if (!force) {
        const latest = await api.posts.get(post.id);
        if (freshness(post, latest) === 'changed') {
          setConflict(latest);
          return;
        }
      }
      setConflict(null);
      const saved = await api.posts.update(post.id, buildUpdate(state, form, baseline.values, post.status));
      onBaseline({ post: { ...post, ...saved }, values: form });
      if (kind === 'schedule' && state.scheduledAtUtc) await api.posts.schedule(post.id, state.scheduledAtUtc);
      toast.success(kind === 'schedule' ? 'Changes saved and post scheduled' : 'Changes saved');
      onLeave();
      router.push(postHref(post.id));
    } catch (e) {
      setApiError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  function loadLatest() {
    if (!conflict) return;
    const values = formFromPost(conflict, fields.timezone);
    fields.setForm(values);
    onBaseline({ post: conflict, values });
    setConflict(null);
    setIssues([]);
  }

  return (
    <>
      <Feedback issues={issues} apiError={apiError} />
      {conflict ? <Conflict latest={conflict} onLoad={loadLatest} onForce={() => void save('save', true)} /> : null}
      <div className="flex flex-wrap gap-2 border-t pt-6">
        <Button loading={busy} onClick={() => void save('save', false)}>
          {busy ? 'Saving…' : 'Save changes'}
        </Button>
        {post.status === 'draft' ? (
          <Button variant="secondary" disabled={busy} onClick={() => void save('schedule', false)}>
            Save and schedule
          </Button>
        ) : null}
        <Button variant="ghost" asChild>
          <Link href={postHref(post.id)}>Cancel</Link>
        </Button>
      </div>
    </>
  );
}

'use client';
import { useState } from 'react';
import { Field, Input } from '@/components/ui/input';
import { EMPTY_FORM, isDirty, type FormValues } from '@/lib/post-edit';
import type { Post, Provider, SocialAccount } from '@/lib/types';
import { AccountChips } from './account-chips';
import { ContentEditor } from './content-editor';
import { CreateActions } from './create-actions';
import { EditActions, type Baseline } from './edit-actions';
import { MediaSection } from './media-section';
import { PreviewsPanel } from './previews-panel';
import { ScheduleFields } from './schedule-fields';
import { useComposerFields } from './use-composer-fields';
import { useUnsavedGuard } from './use-unsaved-guard';
import { useTranslations } from '@/i18n/use-translations';

interface Props {
  accounts: SocialAccount[];
  providers: Provider[];
  /** The post being edited, with the form values it was prefilled from. Absent when composing a new post. */
  edit?: { post: Post; values: FormValues };
}

export function ComposerForm({ accounts, providers, edit }: Props) {
  const t = useTranslations('composer');
  const [baseline, setBaseline] = useState<Baseline | null>(edit ?? null);
  const fields = useComposerFields(edit?.values ?? EMPTY_FORM);
  const { form, patch, state, timezone } = fields;
  const guard = useUnsavedGuard(isDirty(form, baseline?.values ?? EMPTY_FORM));
  const selected = accounts.filter((a) => form.accountIds.includes(a.id));
  const toggle = (id: string) =>
    patch({ accountIds: form.accountIds.includes(id) ? form.accountIds.filter((x) => x !== id) : [...form.accountIds, id] });

  return (
    <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <div className="space-y-6">
        <Field label={t('titleLabel')} htmlFor="post-title" hint={t('titleHint')}>
          <Input id="post-title" value={form.title} onChange={(e) => patch({ title: e.target.value })} maxLength={120} />
        </Field>
        <div className="space-y-2">
          <h2 className="text-sm font-semibold">{t('publishTo')}</h2>
          <AccountChips accounts={accounts} selected={form.accountIds} onToggle={toggle} />
        </div>
        <ContentEditor
          content={form.content}
          overrides={form.overrides}
          selected={selected}
          providers={providers}
          onContent={(content) => patch({ content })}
          onOverride={(id, v) => patch({ overrides: { ...form.overrides, [id]: v } })}
        />
        <MediaSection media={form.media} onChange={(media) => patch({ media })} />
        <ScheduleFields
          date={form.date}
          time={form.time}
          timezone={timezone}
          onDate={(date) => patch({ date })}
          onTime={(time) => patch({ time })}
          note={baseline?.post.status === 'scheduled' ? t('keepSchedule') : undefined}
        />
        {baseline ? (
          <EditActions fields={fields} baseline={baseline} onBaseline={setBaseline} onLeave={guard.allowLeave} accounts={accounts} providers={providers} />
        ) : (
          <CreateActions onLeave={guard.allowLeave} state={state} title={form.title} accounts={accounts} providers={providers} selected={selected} />
        )}
      </div>
      <PreviewsPanel selected={selected} content={form.content} overrides={form.overrides} media={form.media} />
      {guard.dialog}
    </div>
  );
}

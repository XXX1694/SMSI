'use client';
import Link from 'next/link';
import { CalendarClock, Link2, Send, TriangleAlert } from 'lucide-react';
import { useCallback, useState } from 'react';
import { OnboardingChecklist } from '@/components/onboarding-checklist';
import { PostList } from '@/components/post-row';
import { ErrorState, LoadingRows } from '@/components/states';
import { RetryPostDialog } from '@/components/posts/retry-post-dialog';
import { useToast } from '@/components/toast';
import { Section } from '@/components/ui/card';
import { api } from '@/lib/api';
import type { DashboardSummary, Post } from '@/lib/types';
import { useAsync } from '@/hooks';
import { useTranslations } from '@/i18n/use-translations';

const STAT_ICONS = { accounts: Link2, scheduled: CalendarClock, publishedMonth: Send, failed: TriangleAlert } as const;

function Stat({ name, value, tone }: { name: keyof typeof STAT_ICONS; value: number; tone?: 'danger' }) {
  const t = useTranslations('dashboard.stats');
  const Icon = STAT_ICONS[name];
  const alert = tone === 'danger' && value > 0;
  return (
    <div className="glass-card flex flex-col justify-between rounded-xl border p-4">
      <dt className="flex items-center gap-2 text-xs text-muted-foreground">
        <span aria-hidden className={`grid h-7 w-7 shrink-0 place-items-center rounded-md ${alert ? 'bg-danger/10 text-danger' : 'bg-secondary text-secondary-foreground'}`}>
          <Icon className="h-4 w-4" />
        </span>
        <span className="min-w-0">{t(name)}</span>
      </dt>
      <dd className={`mt-2 text-3xl font-semibold tabular-nums tracking-tight ${alert ? 'text-danger' : ''}`}>{value}</dd>
    </div>
  );
}

type SectionName = 'upcoming' | 'drafts' | 'recent' | 'failed';

interface SectionProps {
  name: SectionName;
  posts: Post[];
  href?: string;
  onRetry?: (post: Post) => void;
}

function PostSection({ name, posts, href, onRetry }: SectionProps) {
  const t = useTranslations('dashboard');
  return (
    <Section
      title={t(`sections.${name}.title`)}
      action={
        href ? (
          <Link
            href={href}
            aria-label={t(`sections.${name}.viewAll`)}
            className="-my-3 inline-flex min-h-11 items-center px-1 text-xs text-muted-foreground hover:text-foreground md:-my-2 md:min-h-8"
          >
            {t('viewAll')}
          </Link>
        ) : null
      }
    >
      {posts.length === 0 ? <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">{t(`sections.${name}.empty`)}</p> : <PostList posts={posts} onRetry={onRetry} />}
    </Section>
  );
}

export function DashboardView() {
  const t = useTranslations('dashboard');
  const tp = useTranslations('posts');
  const load = useCallback(async () => {
    const [summary, drafts, failed] = await Promise.all([
      api.dashboard.summary(),
      api.posts.list({ status: 'draft', limit: 5 }),
      api.posts.list({ status: 'failed', limit: 5 }),
    ]);
    return { summary, drafts: drafts.items, failed: failed.items } satisfies { summary: DashboardSummary; drafts: Post[]; failed: Post[] };
  }, []);
  const { data, error, loading, reload } = useAsync(load);
  const toast = useToast();
  const [retryTarget, setRetryTarget] = useState<Post | null>(null);

  if (loading && !data) return <LoadingRows rows={4} />;
  if (error || !data) return <ErrorState error={error} onRetry={reload} />;
  const { summary, drafts, failed } = data;

  return (
    <div className="stagger space-y-10">
      <div>
        <h2 className="sr-only">{t('overview')}</h2>
        <dl className="grid grid-cols-2 gap-3 md:grid-cols-4 md:gap-4">
          <Stat name="accounts" value={summary.connected_accounts} />
          <Stat name="scheduled" value={summary.scheduled_posts} />
          <Stat name="publishedMonth" value={summary.published_this_month} />
          <Stat name="failed" value={summary.failed} tone="danger" />
        </dl>
      </div>
      <OnboardingChecklist connectedAccounts={summary.connected_accounts} />
      <div className="stagger grid grid-cols-[minmax(0,1fr)] gap-10 lg:grid-cols-2">
        <PostSection name="upcoming" posts={summary.upcoming} href="/posts?status=scheduled" />
        <PostSection name="drafts" posts={drafts} href="/posts?status=draft" />
        <PostSection name="recent" posts={summary.recent} href="/posts?status=published" />
        <PostSection name="failed" posts={failed} href="/posts?status=failed" onRetry={setRetryTarget} />
      </div>
      <RetryPostDialog
        postId={retryTarget?.id ?? ''}
        open={retryTarget !== null}
        onOpenChange={(o) => !o && setRetryTarget(null)}
        onRetried={() => {
          toast.success(tp('retryStarted'));
          reload();
        }}
      />
    </div>
  );
}

'use client';
import Link from 'next/link';
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

function Stat({ label, value, tone }: { label: string; value: number; tone?: 'danger' }) {
  return (
    <div className="px-1 py-2 sm:px-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={`mt-1 text-2xl font-semibold tabular-nums ${tone === 'danger' && value > 0 ? 'text-danger' : ''}`}>{value}</dd>
    </div>
  );
}

interface SectionProps {
  title: string;
  posts: Post[];
  empty: string;
  href?: string;
  onRetry?: (post: Post) => void;
}

function PostSection({ title, posts, empty, href, onRetry }: SectionProps) {
  return (
    <Section
      title={title}
      action={
        href ? (
          <Link
            href={href}
            aria-label={`View all ${title.toLowerCase()}`}
            className="-my-3 inline-flex min-h-11 items-center px-1 text-xs text-muted-foreground hover:text-foreground md:-my-2 md:min-h-8"
          >
            View all
          </Link>
        ) : null
      }
    >
      {posts.length === 0 ? <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">{empty}</p> : <PostList posts={posts} onRetry={onRetry} />}
    </Section>
  );
}

export function DashboardView() {
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
        <h2 className="sr-only">Overview</h2>
        <dl className="grid grid-cols-2 gap-x-6 gap-y-2 border-y py-4 md:grid-cols-4">
          <Stat label="Connected accounts" value={summary.connected_accounts} />
          <Stat label="Scheduled" value={summary.scheduled_posts} />
          <Stat label="Published this month" value={summary.published_this_month} />
          <Stat label="Failed" value={summary.failed} tone="danger" />
        </dl>
      </div>
      <OnboardingChecklist connectedAccounts={summary.connected_accounts} />
      <div className="stagger grid gap-10 lg:grid-cols-2">
        <PostSection title="Upcoming" posts={summary.upcoming} empty="Nothing scheduled." href="/posts?status=scheduled" />
        <PostSection title="Drafts" posts={drafts} empty="No drafts." href="/posts?status=draft" />
        <PostSection title="Recently published" posts={summary.recent} empty="Nothing published yet." href="/posts?status=published" />
        <PostSection title="Failed" posts={failed} empty="No failed posts." href="/posts?status=failed" onRetry={setRetryTarget} />
      </div>
      <RetryPostDialog
        postId={retryTarget?.id ?? ''}
        open={retryTarget !== null}
        onOpenChange={(o) => !o && setRetryTarget(null)}
        onRetried={() => {
          toast.success('Retry started');
          reload();
        }}
      />
    </div>
  );
}

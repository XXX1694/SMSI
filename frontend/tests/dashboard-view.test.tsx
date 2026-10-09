import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Post } from '@/lib/types';

const apiMock = vi.hoisted(() => ({
  dashboard: { summary: vi.fn() },
  posts: { list: vi.fn(), retry: vi.fn() },
  approvals: { list: vi.fn() },
  developer: { apiKeys: vi.fn(), mcpConnections: vi.fn() },
}));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
vi.mock('@/components/onboarding-checklist', () => ({ OnboardingChecklist: () => null }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock('@/components/toast', () => ({ useToast: () => toast }));

import { DashboardView } from '@/components/dashboard-view';

const failedPost = {
  id: 'p1', status: 'failed', title: 'Broken launch', content: '', scheduled_at: null, published_at: null, created_at: '2026-10-01T10:00:00Z',
  targets: [{ id: 't1', platform: 'telegram', status: 'failed', content: '' }],
} as unknown as Post;
const empty = { items: [], next_cursor: null };

beforeEach(() => {
  vi.clearAllMocks();
  apiMock.dashboard.summary.mockResolvedValue({ connected_accounts: 1, scheduled_posts: 0, published_this_month: 0, failed: 1, upcoming: [], recent: [] });
  apiMock.posts.list.mockImplementation(async (q: { status?: string }) => (q.status === 'failed' ? { items: [failedPost], next_cursor: null } : empty));
  apiMock.posts.retry.mockResolvedValue(failedPost);
});

describe('DashboardView failed posts', () => {
  it('asks before retrying and does nothing on Cancel', async () => {
    render(<DashboardView />);
    await userEvent.click(await screen.findByRole('button', { name: 'Retry Broken launch' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Retry failed accounts?')).toBeInTheDocument();
    expect(apiMock.posts.retry).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(apiMock.posts.retry).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  it('retries only after Confirm, then reloads', async () => {
    render(<DashboardView />);
    await userEvent.click(await screen.findByRole('button', { name: 'Retry Broken launch' }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Retry' }));
    expect(apiMock.posts.retry).toHaveBeenCalledWith('p1');
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Retry started'));
    await waitFor(() => expect(apiMock.dashboard.summary).toHaveBeenCalledTimes(2));
  });

  it('shows a refusal inside the dialog and keeps it open', async () => {
    apiMock.posts.retry.mockRejectedValue(new Error('Approval needed'));
    render(<DashboardView />);
    await userEvent.click(await screen.findByRole('button', { name: 'Retry Broken launch' }));
    const dialog = await screen.findByRole('dialog');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Retry' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Approval needed');
    expect(toast.success).not.toHaveBeenCalled();
  });

  it('gives each View all link a distinct name and the stats row a heading', async () => {
    render(<DashboardView />);
    expect(await screen.findByRole('link', { name: 'View all failed' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'View all drafts' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Overview' })).toBeInTheDocument();
  });
});

describe('DashboardView first-load stagger', () => {
  it('staggers the sections on the first load in a tab only', async () => {
    window.sessionStorage.clear();
    const first = render(<DashboardView />);
    await screen.findByRole('button', { name: 'Retry Broken launch' });
    expect(first.container.querySelector('.stagger')).not.toBeNull();
    first.unmount();

    render(<DashboardView />);
    await screen.findByRole('button', { name: 'Retry Broken launch' });
    expect(document.querySelector('.stagger')).toBeNull();
  });
});

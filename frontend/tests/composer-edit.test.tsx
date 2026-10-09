import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({
  social: { providers: vi.fn(), accounts: vi.fn() },
  posts: { get: vi.fn(), update: vi.fn(), schedule: vi.fn(), create: vi.fn(), publish: vi.fn() },
  media: { upload: vi.fn(), list: vi.fn() },
}));
const nav = vi.hoisted(() => ({ push: vi.fn() }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
vi.mock('@/components/toast', () => ({ useToast: () => toast }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ push: nav.push, replace: vi.fn() }) }));
vi.mock('next/link', () => ({
  default: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

import { ComposerView } from '@/components/composer/composer-view';
import type { Post, PostTarget } from '@/lib/types';

const provider = (id: string) => ({
  id, name: id, configured: true, unsupported: false, available: true,
  capabilities: { canPublishText: true, canPublishImage: true, canPublishVideo: true, canSchedule: true, canDelete: true, canAnalytics: false, maxTextLength: 500, maxMediaCount: 4, requiresApproval: false, notes: '', connectMethod: 'oauth' },
});
const account = (id: string, p: string) => ({ id, provider: p, username: id, display_name: `${p}-name`, avatar_url: null, status: 'active' as const, scopes: [], connected_at: '2026-01-01T00:00:00Z' });
const target = (account_id: string, platform: string, content: string): PostTarget => ({ id: `t-${account_id}`, social_account_id: account_id, platform, content, status: 'pending' as const, external_url: null, published_at: null, error_code: null, error_message: null, attempt_count: 0 });

const scheduled = (over: Partial<Post> = {}): Post => ({
  id: 'p1', title: 'Launch', content: 'Base text', status: 'scheduled', scheduled_at: '2099-12-01T22:30:00Z', published_at: null, created_by: 'user',
  created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-02T00:00:00Z', media: [],
  targets: [target('a1', 'telegram', 'Base text'), target('a2', 'mastodon', 'Short text')], attempts: [], ...over,
} as Post);

beforeEach(() => {
  vi.clearAllMocks();
  apiMock.social.providers.mockResolvedValue([provider('telegram'), provider('mastodon')]);
  apiMock.social.accounts.mockResolvedValue([account('a1', 'telegram'), account('a2', 'mastodon')]);
  apiMock.posts.get.mockResolvedValue(scheduled());
  apiMock.posts.update.mockResolvedValue(scheduled({ updated_at: '2026-10-03T00:00:00Z' }));
});

const open = async () => {
  const view = render(<ComposerView postId="p1" />);
  await screen.findByLabelText('Post content');
  return view;
};
const type = (value: string) => fireEvent.change(screen.getByLabelText('Post content'), { target: { value } });

describe('ComposerView in edit mode', () => {
  it('prefills title, text, accounts, overrides and the schedule from the post', async () => {
    await open();
    expect(screen.getByLabelText(/Title/)).toHaveValue('Launch');
    expect(screen.getByLabelText('Post content')).toHaveValue('Base text');
    expect(screen.getByLabelText('Date')).toHaveValue('2099-12-01');
    expect(screen.getByLabelText('Time')).toHaveValue('22:30');
    for (const name of [/telegram-name/, /mastodon-name/]) expect(screen.getByRole('button', { name })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('tab', { name: /mastodon/i })).toHaveTextContent('•');
    expect(apiMock.posts.get).toHaveBeenCalledWith('p1');
  });

  it('saves with PATCH, keeps the time it was not asked to change, toasts and returns to the post', async () => {
    await open();
    type('Edited text');
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(apiMock.posts.update).toHaveBeenCalled());
    const [id, body] = apiMock.posts.update.mock.calls[0] ?? [];
    expect(id).toBe('p1');
    expect(body).toMatchObject({ content: 'Edited text', title: 'Launch', social_account_ids: ['a1', 'a2'] });
    expect(body.targets).toEqual([{ social_account_id: 'a1', content: 'Edited text' }, { social_account_id: 'a2', content: 'Short text' }]);
    expect(body).not.toHaveProperty('scheduled_at');
    expect(toast.success).toHaveBeenCalledWith('Changes saved');
    expect(nav.push).toHaveBeenCalledWith('/posts/p1');
  });

  it('sends the new time in UTC, taken from the Settings timezone, when it is changed', async () => {
    await open();
    fireEvent.change(screen.getByLabelText('Time'), { target: { value: '23:45' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(apiMock.posts.update).toHaveBeenCalled());
    expect(apiMock.posts.update.mock.calls[0]?.[1].scheduled_at).toBe('2099-12-01T23:45:00Z');
  });

  it('refuses a new time in the past and does not call the API', async () => {
    await open();
    fireEvent.change(screen.getByLabelText('Date'), { target: { value: '2020-01-01' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText(/at least 1 minute from now/)).toBeInTheDocument();
    expect(apiMock.posts.update).not.toHaveBeenCalled();
  });

  it('schedules a draft after saving when asked', async () => {
    apiMock.posts.get.mockResolvedValue(scheduled({ status: 'draft', scheduled_at: null }));
    await open();
    expect(screen.getByLabelText('Date')).toHaveValue('');
    fireEvent.change(screen.getByLabelText('Date'), { target: { value: '2099-12-05' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save and schedule' }));
    await waitFor(() => expect(apiMock.posts.schedule).toHaveBeenCalledWith('p1', '2099-12-05T09:00:00Z'));
    expect(apiMock.posts.update.mock.calls[0]?.[1]).not.toHaveProperty('scheduled_at');
  });

  it('shows the API error and stays on the page when saving fails', async () => {
    const { ApiError } = await import('@/lib/api');
    apiMock.posts.update.mockRejectedValue(new ApiError(409, 'INVALID_STATE_TRANSITION', 'post in status publishing cannot be edited'));
    await open();
    type('x');
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText('post in status publishing cannot be edited')).toBeInTheDocument();
    expect(nav.push).not.toHaveBeenCalled();
  });
});

describe('ComposerView conflicts', () => {
  const newer = () => scheduled({ updated_at: '2026-10-09T00:00:00Z', content: 'Changed elsewhere', targets: [target('a1', 'telegram', 'Changed elsewhere')] });

  it('stops before saving when the post changed since it was opened, and offers the latest version', async () => {
    await open();
    type('Mine');
    apiMock.posts.get.mockResolvedValue(newer());
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText('This post changed since you opened it.')).toBeInTheDocument();
    expect(apiMock.posts.update).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Load the latest version' }));
    expect(screen.getByLabelText('Post content')).toHaveValue('Changed elsewhere');
    expect(screen.queryByText('This post changed since you opened it.')).not.toBeInTheDocument();
  });

  it('lets the user overwrite on purpose', async () => {
    await open();
    type('Mine');
    apiMock.posts.get.mockResolvedValue(newer());
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Save mine anyway' }));
    await waitFor(() => expect(apiMock.posts.update).toHaveBeenCalled());
    expect(apiMock.posts.update.mock.calls[0]?.[1].content).toBe('Mine');
  });

  it('does not offer to overwrite a post that is no longer editable', async () => {
    await open();
    type('Mine');
    apiMock.posts.get.mockResolvedValue(scheduled({ status: 'publishing' }));
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    expect(await screen.findByText(/cannot be edited/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save mine anyway' })).not.toBeInTheDocument();
  });
});

describe('ComposerView disabled states', () => {
  it.each([
    ['published', /already published/],
    ['publishing', /being published right now/],
    ['cancelled', /was cancelled/],
    ['failed', /This post is failed/],
  ] as const)('explains why a %s post cannot be edited and shows no form', async (status, reason) => {
    apiMock.posts.get.mockResolvedValue(scheduled({ status }));
    render(<ComposerView postId="p1" />);
    expect(await screen.findByText('This post cannot be edited')).toBeInTheDocument();
    expect(screen.getByText(reason)).toBeInTheDocument();
    expect(screen.queryByLabelText('Post content')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Back to the post' })).toHaveAttribute('href', '/posts/p1');
  });

  it('shows a titled error when the post cannot be loaded', async () => {
    const { ApiError } = await import('@/lib/api');
    apiMock.posts.get.mockRejectedValue(new ApiError(404, 'NOT_FOUND', 'Post not found'));
    render(<ComposerView postId="p1" />);
    expect(await screen.findByText('Could not load this post')).toBeInTheDocument();
  });
});

describe('ComposerView unsaved changes guard', () => {
  it('lets a clean form leave without asking', async () => {
    await open();
    const stop = (e: Event) => e.preventDefault(); // jsdom cannot navigate
    window.addEventListener('click', stop);
    await userEvent.click(screen.getByRole('link', { name: 'Cancel' }));
    window.removeEventListener('click', stop);
    expect(screen.queryByText('Discard unsaved changes?')).not.toBeInTheDocument();
  });

  it('asks before an in-app link drops edits; staying keeps them, discarding navigates', async () => {
    await open();
    type('Half written');
    await userEvent.click(screen.getByRole('link', { name: 'Cancel' }));
    expect(await screen.findByText('Discard unsaved changes?')).toBeInTheDocument();
    expect(nav.push).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByText('Discard unsaved changes?')).not.toBeInTheDocument());
    expect(screen.getByLabelText('Post content')).toHaveValue('Half written');
    await userEvent.click(screen.getByRole('link', { name: 'Cancel' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Discard changes' }));
    expect(nav.push).toHaveBeenCalledWith('/posts/p1');
  });

  it('asks the browser to confirm a reload only while there are unsaved edits', async () => {
    await open();
    const clean = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(clean);
    expect(clean.defaultPrevented).toBe(false);
    type('Half written');
    const dirty = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(dirty);
    expect(dirty.defaultPrevented).toBe(true);
  });

  it('does not nag after a successful save', async () => {
    await open();
    type('Done');
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(nav.push).toHaveBeenCalledWith('/posts/p1'));
    const ev = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(false);
  });
});

describe('ComposerView review fixes', () => {
  it('Discard hands the router a path without the deploy base path', async () => {
    vi.stubEnv('NEXT_PUBLIC_BASE_PATH', '/steerpost/demo');
    try {
      await open();
      type('Half written');
      const a = document.createElement('a');
      a.href = '/steerpost/demo/posts/view?id=p1';
      a.textContent = 'Back to post';
      document.body.appendChild(a);
      await userEvent.click(a);
      await userEvent.click(await screen.findByRole('button', { name: 'Discard changes' }));
      expect(nav.push).toHaveBeenCalledWith('/posts/view?id=p1');
      a.remove();
    } finally {
      vi.unstubAllEnvs();
    }
  });

  it('says the draft was saved but not scheduled when /schedule fails, and leaves the form clean', async () => {
    apiMock.posts.get.mockResolvedValue(scheduled({ status: 'draft', scheduled_at: null }));
    const { ApiError } = await import('@/lib/api');
    apiMock.posts.schedule.mockRejectedValue(new ApiError(403, 'QUOTA_EXCEEDED', 'Monthly limit reached.'));
    await open();
    type('Edited draft');
    fireEvent.change(screen.getByLabelText('Date'), { target: { value: '2099-12-05' } });
    await userEvent.click(screen.getByRole('button', { name: 'Save and schedule' }));
    expect(await screen.findByText('Changes saved, but the post was not scheduled: Monthly limit reached.')).toBeInTheDocument();
    expect(nav.push).not.toHaveBeenCalled();
    const ev = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(false);
  });

  it('guards unsaved text on a new post too', async () => {
    render(<ComposerView />);
    await screen.findByLabelText('Post content');
    type('Draft idea');
    const ev = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
  });
});

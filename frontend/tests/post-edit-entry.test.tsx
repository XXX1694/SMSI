import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({ posts: { get: vi.fn() } }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
vi.mock('@/components/toast', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn() }) }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ replace: vi.fn(), push: vi.fn() }) }));
vi.mock('next/link', () => ({
  default: ({ href, children, ...rest }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

import { PostRow } from '@/components/post-row';
import { PostDetail } from '@/components/posts/post-detail';
import type { Post, PostStatus } from '@/lib/types';

const post = (status: PostStatus): Post => ({
  id: 'p1', title: 'Webinar', status, scheduled_at: null, published_at: null, created_by: 'user', created_at: '2026-10-07T07:00:00Z',
  targets: [{ id: 't1', social_account_id: 'a1', platform: 'telegram', content: 'Hi', status: 'pending', external_url: null, published_at: null, error_code: null, error_message: null, attempt_count: 0 }],
  attempts: [],
});

beforeEach(() => apiMock.posts.get.mockReset());

describe('Edit entry points', () => {
  it.each(['draft', 'scheduled'] as const)('post detail links a %s post to the composer', async (status) => {
    apiMock.posts.get.mockResolvedValue(post(status));
    render(<PostDetail id="p1" />);
    const edit = await screen.findByRole('link', { name: 'Edit' });
    expect(edit).toHaveAttribute('href', '/compose?post=p1');
    expect(screen.queryByText(/cannot be edited|can be edited/)).not.toBeInTheDocument();
  });

  it('post detail shows a disabled Edit with the reason for a published post', async () => {
    apiMock.posts.get.mockResolvedValue(post('published'));
    render(<PostDetail id="p1" />);
    const edit = await screen.findByRole('button', { name: 'Edit' });
    expect(edit).toBeDisabled();
    expect(edit).toHaveAccessibleDescription(/already published/);
    expect(screen.queryByRole('link', { name: 'Edit' })).not.toBeInTheDocument();
  });

  it('the posts list offers Edit only where the API allows it', () => {
    const { rerender } = render(<ul><PostRow post={post('draft')} /></ul>);
    expect(screen.getByRole('link', { name: 'Edit Webinar' })).toHaveAttribute('href', '/compose?post=p1');
    rerender(<ul><PostRow post={post('failed')} /></ul>);
    expect(screen.queryByRole('link', { name: /^Edit/ })).not.toBeInTheDocument();
  });
});

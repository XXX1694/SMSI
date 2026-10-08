import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({ posts: { get: vi.fn() } }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
vi.mock('@/components/toast', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn() }) }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ replace: vi.fn(), push: vi.fn() }) }));

import { PostDetail } from '@/components/posts/post-detail';
import type { Post } from '@/lib/types';

const failed: Post = {
  id: 'p1', title: 'Webinar', status: 'failed', scheduled_at: null, published_at: null, created_by: 'user', created_at: '2026-10-07T07:00:00Z',
  targets: [{
    id: 't1', social_account_id: 'a1', platform: 'telegram', content: 'Hi', status: 'failed', external_url: null, published_at: null,
    error_code: 'PROVIDER_ERROR', error_message: 'Telegram: Bad Request: chat not found', attempt_count: 1,
  }],
  attempts: [],
};

beforeEach(() => apiMock.posts.get.mockResolvedValue(failed));

describe('PostDetail failed target', () => {
  it('explains the failure in a sentence and never shows the raw error code', async () => {
    const { container } = render(<PostDetail id="p1" />);
    expect(await screen.findByText(/chat not found/)).toBeInTheDocument();
    expect(container.textContent).not.toContain('PROVIDER_ERROR');
    expect(container.textContent).toContain('The network could not publish the post.');
  });
});

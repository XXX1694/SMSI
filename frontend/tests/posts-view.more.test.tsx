import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({ posts: { list: vi.fn() } }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ replace: vi.fn() }), useSearchParams: () => new URLSearchParams('') }));
vi.mock('next/link', () => ({ default: ({ children }: { children: React.ReactNode }) => <span>{children}</span> }));

import { PostsView } from '@/components/posts/posts-view';
import { ApiError } from '@/lib/api';
import type { Post } from '@/lib/types';

const post = (id: string, title: string): Post => ({
  id, title, status: 'draft', scheduled_at: null, published_at: null, created_by: 'user', created_at: '2026-10-01T10:00:00Z', targets: [],
});

beforeEach(() => apiMock.posts.list.mockReset());

describe('PostsView "Load more"', () => {
  it('keeps the loaded list and offers an inline retry when the next page fails', async () => {
    apiMock.posts.list
      .mockResolvedValueOnce({ items: [post('1', 'First post')], next_cursor: 'c1' })
      .mockRejectedValueOnce(new ApiError(500, 'INTERNAL', 'Request failed (500).'))
      .mockResolvedValueOnce({ items: [post('2', 'Second post')], next_cursor: null });
    render(<PostsView />);
    await userEvent.click(await screen.findByRole('button', { name: 'Load more' }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(screen.getByText('First post')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(screen.getByText('Second post')).toBeInTheDocument());
    expect(screen.getByText('First post')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});

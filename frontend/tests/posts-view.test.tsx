import { render, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PostsView } from '@/components/posts/posts-view';

const apiMock = vi.hoisted(() => ({ posts: { list: vi.fn() } }));
vi.mock('@/lib/api', () => ({ api: apiMock, ApiError: class ApiError extends Error {} }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'Pacific/Kiritimati' }) }));
vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: vi.fn() }),
  useSearchParams: () => new URLSearchParams('from=2026-10-01&to=2026-10-01'),
}));
vi.mock('next/link', () => ({ default: ({ children }: { children: React.ReactNode }) => <span>{children}</span> }));

beforeEach(() => apiMock.posts.list.mockReset());

describe('PostsView date filter', () => {
  it('uses the timezone from Settings, not the browser one, for the day bounds', async () => {
    apiMock.posts.list.mockResolvedValue({ items: [], next_cursor: null });
    render(<PostsView />);
    await waitFor(() => expect(apiMock.posts.list).toHaveBeenCalled());
    // Kiritimati is UTC+14: local 2026-10-01 00:00 is 2026-09-30T10:00:00Z, and 23:59:59 is 2026-10-01T09:59:59Z.
    expect(apiMock.posts.list.mock.calls[0]?.[0]).toMatchObject({
      from: '2026-09-30T10:00:00Z',
      to: '2026-10-01T09:59:59Z',
    });
  });
});

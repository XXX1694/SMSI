import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({ posts: { list: vi.fn() } }));
vi.mock('@/lib/api', () => ({ api: apiMock, ApiError: class ApiError extends Error {} }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
vi.mock('next/link', () => ({ default: ({ children, href }: { children: React.ReactNode; href: string }) => <a href={href}>{children}</a> }));

import { CalendarViewPage } from '@/components/calendar-view';

const today = new Date().toISOString().slice(0, 10);
const post = (i: number) => ({ id: `p${i}`, status: 'scheduled', title: `Post ${i}`, content: '', scheduled_at: `${today}T0${i}:00:00Z`, published_at: null, created_at: `${today}T00:00:00Z`, targets: [] });

describe('Calendar month view', () => {
  it('makes "+N more" a button that opens the day view', async () => {
    apiMock.posts.list.mockResolvedValue({ items: [1, 2, 3, 4, 5].map(post), next_cursor: null });
    render(<CalendarViewPage />);
    await userEvent.click(await screen.findByRole('button', { name: /\+2 more/ }));
    expect(screen.getByRole('button', { name: 'day' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getAllByRole('link', { name: /Post \d/ })).toHaveLength(5);
  });
});

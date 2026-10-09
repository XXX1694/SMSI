import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '@/lib/api';
import type { UsageReport } from '@/lib/types';

const apiMock = vi.hoisted(() => ({ account: { usage: vi.fn() } }));
vi.mock('@/lib/api', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    status: number;
    code: string;
    requestId: string | null = null;
    fields = {};
    constructor(status: number, code: string, message: string) {
      super(message);
      this.status = status;
      this.code = code;
    }
  },
}));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));

import { UsageCard } from '@/components/usage-card';

const report = (over: Partial<UsageReport['quotas']> = {}): UsageReport => ({
  plan: 'free',
  period_start: '2026-10-01T00:00:00Z',
  period_end: '2026-11-01T00:00:00Z',
  quotas: {
    connected_accounts: { used: 2, limit: 5 },
    scheduled_posts_month: { used: 12, limit: 60 },
    media_bytes: { used: 5 * 1024 * 1024, limit: 500 * 1024 * 1024 },
    agent_requests_per_minute: { limit: 120 },
    ...over,
  },
});

beforeEach(() => apiMock.account.usage.mockReset());

describe('UsageCard', () => {
  it('shows used against the limit for each quota and the agent rate', async () => {
    apiMock.account.usage.mockResolvedValue(report());
    render(<UsageCard />);
    expect(await screen.findByText('2 of 5')).toBeInTheDocument();
    expect(screen.getByText('12 of 60')).toBeInTheDocument();
    expect(screen.getByText('5.0 MB of 500 MB')).toBeInTheDocument();
    expect(screen.getByRole('progressbar', { name: 'Connected accounts' })).toHaveAttribute('aria-valuenow', '2');
    expect(screen.getByText(/up to 120 requests per minute/)).toBeInTheDocument();
    expect(screen.queryByText(/limit reached/)).not.toBeInTheDocument();
  });

  it('says so plainly when a limit is reached and when there is none', async () => {
    apiMock.account.usage.mockResolvedValue(report({ connected_accounts: { used: 5, limit: 5 }, scheduled_posts_month: { used: 9, limit: -1 }, agent_requests_per_minute: { limit: -1 } }));
    render(<UsageCard />);
    expect(await screen.findByText(/5 of 5 · limit reached/)).toBeInTheDocument();
    expect(screen.getByText(/9 · no limit/)).toBeInTheDocument();
    expect(screen.getByText(/no request limit/)).toBeInTheDocument();
    expect(screen.queryByRole('progressbar', { name: 'Posts this month' })).not.toBeInTheDocument();
  });

  it('offers a retry when the usage cannot be loaded', async () => {
    apiMock.account.usage.mockRejectedValueOnce(new ApiError(500, 'INTERNAL', 'down'));
    render(<UsageCard />);
    expect(await screen.findByText('Could not load your usage')).toBeInTheDocument();
  });
});

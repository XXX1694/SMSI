import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { DataExport } from '@/lib/types';

const apiMock = vi.hoisted(() => ({ account: { exports: { list: vi.fn(), request: vi.fn(), link: vi.fn() } } }));
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

import { ApiError } from '@/lib/api';
import { DataExportCard } from '@/components/data-export';

const exp = (over: Partial<DataExport> = {}): DataExport => ({
  id: 'e1',
  status: 'ready',
  size_bytes: 5 * 1024 * 1024,
  error_code: null,
  created_at: '2026-10-08T10:00:00Z',
  expires_at: '2026-10-15T10:00:00Z',
  ...over,
});

beforeEach(() => {
  Object.values(apiMock.account.exports).forEach((f) => f.mockReset());
});

describe('DataExportCard', () => {
  it('shows a loading state, then the empty state with the request button', async () => {
    apiMock.account.exports.list.mockResolvedValue([]);
    render(<DataExportCard />);
    expect(screen.getByRole('status')).toHaveTextContent('Loading…');
    expect(await screen.findByRole('button', { name: 'Request export' })).toBeEnabled();
    expect(screen.getByText(/Passwords, keys and tokens are never included/)).toBeInTheDocument();
  });

  it('requests an export and shows that it is being prepared', async () => {
    apiMock.account.exports.list.mockResolvedValueOnce([]).mockResolvedValue([exp({ status: 'pending', size_bytes: 0, expires_at: null })]);
    apiMock.account.exports.request.mockResolvedValue(exp({ status: 'pending' }));
    render(<DataExportCard />);
    await userEvent.click(await screen.findByRole('button', { name: 'Request export' }));
    expect(await screen.findByText(/Preparing your export/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Request export' })).not.toBeInTheDocument();
  });

  it('offers the download of a ready export and says when it is deleted', async () => {
    apiMock.account.exports.list.mockResolvedValue([exp()]);
    apiMock.account.exports.link.mockResolvedValue({ ...exp(), url: 'https://files.test/x.zip', url_expires_at: '2026-10-08T10:05:00Z' });
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    render(<DataExportCard />);
    expect(await screen.findByText(/5\.0 MB\) is ready/)).toBeInTheDocument();
    expect(screen.getByText(/deleted on/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /Download ZIP/ }));
    await waitFor(() => expect(click).toHaveBeenCalled());
    expect(apiMock.account.exports.link).toHaveBeenCalledWith('e1');
    click.mockRestore();
  });

  it('explains a failed and an expired export, and lets the user try again', async () => {
    apiMock.account.exports.list.mockResolvedValue([exp({ status: 'failed', size_bytes: 0, expires_at: null, error_code: 'build_failed' })]);
    const { unmount } = render(<DataExportCard />);
    expect(await screen.findByText(/could not be built/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Request export' })).toBeEnabled();
    unmount();
    apiMock.account.exports.list.mockResolvedValue([exp({ status: 'expired' })]);
    render(<DataExportCard />);
    expect(await screen.findByText(/expired and was deleted/)).toBeInTheDocument();
  });

  it('shows the server reason when the request is refused (cooldown)', async () => {
    apiMock.account.exports.list.mockResolvedValue([]);
    apiMock.account.exports.request.mockRejectedValue(new ApiError(429, 'RATE_LIMITED', 'you exported your data less than 24 hours ago; try again later'));
    render(<DataExportCard />);
    await userEvent.click(await screen.findByRole('button', { name: 'Request export' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/less than 24 hours ago/);
  });

  it('shows a retryable error when the list cannot be loaded', async () => {
    apiMock.account.exports.list.mockRejectedValueOnce(new ApiError(500, 'INTERNAL', 'x')).mockResolvedValue([]);
    render(<DataExportCard />);
    expect(await screen.findByText('Could not load your exports')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('button', { name: 'Request export' })).toBeInTheDocument();
  });
});

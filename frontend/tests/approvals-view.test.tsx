import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApprovalsView } from '@/components/approvals/approvals-view';
import { ApiError } from '@/lib/api';
import type { Approval } from '@/lib/types';

const apiMock = vi.hoisted(() => ({ approvals: { list: vi.fn(), approve: vi.fn(), deny: vi.fn() } }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock('@/lib/api', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    constructor(
      readonly status: number,
      readonly code: string,
      message: string,
      readonly requestId: string | null = null,
    ) {
      super(message);
    }
  },
}));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
vi.mock('@/components/toast', () => ({ useToast: () => toast }));

const soon = () => new Date(Date.now() + 8 * 60_000).toISOString();
const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString();

const publish: Approval = {
  id: 'ap-1', action: 'post.publish', resource_type: 'post', resource_id: 'p1', actor_label: 'MCP: Claude Desktop', status: 'pending',
  summary: { title: 'Launch day', content: 'We are live.', platforms: ['linkedin', 'telegram'] },
  created_at: ago(2), expires_at: soon(), decided_at: null,
};
const disconnect: Approval = {
  id: 'ap-2', action: 'social_account.disconnect', resource_type: 'social_account', resource_id: 'a1', actor_label: 'CI publisher', status: 'pending',
  summary: { provider: 'linkedin', username: 'alex' }, created_at: ago(1), expires_at: soon(), decided_at: null,
};
const page = (items: Approval[]) => ({ items, next_cursor: null });

beforeEach(() => {
  apiMock.approvals.list.mockReset();
  apiMock.approvals.approve.mockReset().mockResolvedValue({});
  apiMock.approvals.deny.mockReset().mockResolvedValue({});
  toast.success.mockReset();
  toast.error.mockReset();
});

describe('ApprovalsView', () => {
  it('shows a loading state, then the waiting requests with what they would do and who asked', async () => {
    apiMock.approvals.list.mockReturnValue(new Promise(() => undefined));
    const { unmount } = render(<ApprovalsView />);
    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument();
    unmount();

    apiMock.approvals.list.mockResolvedValue(page([publish, disconnect]));
    render(<ApprovalsView />);
    expect(await screen.findByText('Publish now')).toBeInTheDocument();
    expect(screen.getByText('MCP: Claude Desktop')).toBeInTheDocument();
    expect(screen.getByText('Launch day')).toBeInTheDocument();
    expect(screen.getByText('We are live.')).toBeInTheDocument();
    expect(screen.getByText('linkedin, telegram')).toBeInTheDocument();
    expect(screen.getByText('Disconnect account')).toBeInTheDocument();
    expect(screen.getAllByText('8 min left')).toHaveLength(2);
    expect(apiMock.approvals.list).toHaveBeenCalledWith('pending', 50);
  });

  it('explains the empty state and what to do next', async () => {
    apiMock.approvals.list.mockResolvedValue(page([]));
    render(<ApprovalsView />);
    expect(await screen.findByText('Nothing is waiting for you')).toBeInTheDocument();
    expect(screen.getByText(/nothing happens until you decide/i)).toBeInTheDocument();
  });

  it('shows the error with a retry that reloads', async () => {
    apiMock.approvals.list.mockImplementationOnce(() => Promise.reject(new ApiError(500, 'INTERNAL', 'The server is down.'))).mockResolvedValueOnce(page([]));
    render(<ApprovalsView />);
    expect(await screen.findByRole('alert')).toHaveTextContent('The server is down.');
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText('Nothing is waiting for you')).toBeInTheDocument();
  });

  it('approves one request, tells the owner the agent can retry, and reloads the list', async () => {
    apiMock.approvals.list.mockResolvedValueOnce(page([publish, disconnect])).mockResolvedValueOnce(page([disconnect]));
    render(<ApprovalsView />);
    await userEvent.click(await screen.findByRole('button', { name: 'Approve: Publish now' }));
    expect(apiMock.approvals.approve).toHaveBeenCalledWith('ap-1');
    await waitFor(() => expect(screen.queryByText('Launch day')).not.toBeInTheDocument());
    expect(toast.success).toHaveBeenCalledWith(expect.stringContaining('Approved: Publish now'));
    expect(screen.getByText('Disconnect account')).toBeInTheDocument();
  });

  it('denies a request', async () => {
    apiMock.approvals.list.mockResolvedValueOnce(page([disconnect])).mockResolvedValueOnce(page([]));
    render(<ApprovalsView />);
    await userEvent.click(await screen.findByRole('button', { name: 'Deny: Disconnect account' }));
    expect(apiMock.approvals.deny).toHaveBeenCalledWith('ap-2');
    expect(apiMock.approvals.approve).not.toHaveBeenCalled();
    expect(await screen.findByText('Nothing is waiting for you')).toBeInTheDocument();
    expect(toast.success).toHaveBeenCalledWith('Denied: Disconnect account.');
  });

  it('keeps the list and shows the reason when deciding fails (for example it expired meanwhile)', async () => {
    apiMock.approvals.approve.mockImplementation(() => Promise.reject(new Error('this approval is no longer pending')));
    apiMock.approvals.list.mockResolvedValue(page([publish]));
    render(<ApprovalsView />);
    await userEvent.click(await screen.findByRole('button', { name: 'Approve: Publish now' }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('this approval is no longer pending'));
    expect(screen.getByText('Launch day')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Approve: Publish now' })).toBeEnabled();
  });

  it('History lists decided and expired requests without buttons', async () => {
    const denied: Approval = { ...disconnect, id: 'ap-3', status: 'denied', decided_at: ago(5) };
    const expired: Approval = { ...publish, id: 'ap-4', status: 'pending', expires_at: ago(1) };
    apiMock.approvals.list.mockResolvedValueOnce(page([publish])).mockResolvedValueOnce(page([denied, expired]));
    render(<ApprovalsView />);
    await screen.findByText('Publish now');
    await userEvent.click(screen.getByRole('button', { name: 'History' }));
    expect(await screen.findByText('Denied')).toBeInTheDocument();
    expect(screen.getByText('Expired')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^(Approve|Deny):/ })).not.toBeInTheDocument();
    expect(apiMock.approvals.list).toHaveBeenLastCalledWith('all', 50);
    expect(within(screen.getByRole('group', { name: 'Show approvals' })).getByRole('button', { name: 'History' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('clamps long text and lets the owner read all of it, including per-network text and media', async () => {
    const long = 'A long post. '.repeat(40);
    const wide: Approval = {
      ...publish,
      summary: { title: 'Big', content: long, targets: [{ platform: 'telegram', content: '<b>raw</b> text' }], media: { count: 1, images: 1, videos: 0 } },
    };
    apiMock.approvals.list.mockResolvedValue(page([wide]));
    render(<ApprovalsView />);
    const toggle = await screen.findByRole('button', { name: 'Show full text' });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getByText(long.trim())).toHaveClass('line-clamp-4');
    expect(screen.getByText('Text on telegram')).toBeInTheDocument();
    expect(screen.getByText('<b>raw</b> text')).toBeInTheDocument(); // shown as text, never as markup
    expect(screen.getByText('1 image')).toBeInTheDocument();
    await userEvent.click(toggle);
    expect(screen.getByRole('button', { name: 'Show less' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText(long.trim())).not.toHaveClass('line-clamp-4');
  });

  it('marks an irreversible action in danger styling so it is not mistaken for a routine one', async () => {
    apiMock.approvals.list.mockResolvedValue(page([disconnect]));
    render(<ApprovalsView />);
    const approve = await screen.findByRole('button', { name: 'Approve: Disconnect account' });
    expect(approve.className).toContain('bg-danger');
  });
});

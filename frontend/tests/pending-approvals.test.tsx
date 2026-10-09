import { act, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { notifyApprovalsChanged, usePendingApprovals } from '@/components/approvals/use-pending-approvals';

const apiMock = vi.hoisted(() => ({ approvals: { list: vi.fn() } }));
vi.mock('@/lib/api', () => ({ api: apiMock }));

function Badge() {
  const n = usePendingApprovals();
  return <span data-testid="n">{n === null ? 'unknown' : n}</span>;
}

// Braces matter: a beforeEach that returns the mock makes vitest call it again as a cleanup function.
beforeEach(() => {
  apiMock.approvals.list.mockReset();
});

describe('usePendingApprovals', () => {
  it('counts waiting approvals and recounts on focus and after a decision', async () => {
    apiMock.approvals.list.mockResolvedValue({ items: [{}, {}], next_cursor: null });
    render(<Badge />);
    await waitFor(() => expect(screen.getByTestId('n')).toHaveTextContent('2'));
    expect(apiMock.approvals.list).toHaveBeenCalledWith('pending', 100);

    apiMock.approvals.list.mockResolvedValue({ items: [{}], next_cursor: null });
    act(() => void window.dispatchEvent(new Event('focus')));
    await waitFor(() => expect(screen.getByTestId('n')).toHaveTextContent('1'));

    apiMock.approvals.list.mockResolvedValue({ items: [], next_cursor: null });
    act(() => notifyApprovalsChanged());
    await waitFor(() => expect(screen.getByTestId('n')).toHaveTextContent('0'));
  });

  it('shows no number when the count cannot be read, instead of a wrong one', async () => {
    apiMock.approvals.list.mockRejectedValue(new Error('down'));
    render(<Badge />);
    await waitFor(() => expect(apiMock.approvals.list).toHaveBeenCalled());
    expect(screen.getByTestId('n')).toHaveTextContent('unknown');
  });
});

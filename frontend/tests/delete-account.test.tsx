import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Me } from '@/lib/types';

const apiMock = vi.hoisted(() => ({
  account: { requestDeletion: vi.fn(), cancelDeletion: vi.fn() },
  auth: { signInProviders: vi.fn(), socialStartUrl: vi.fn() },
}));
vi.mock('next/navigation', () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock('@/lib/api', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    status: number;
    code: string;
    requestId: string | null = null;
    fields: Record<string, string>;
    constructor(status: number, code: string, message: string, requestId: string | null = null, fields: Record<string, string> = {}) {
      super(message);
      this.status = status;
      this.code = code;
      this.fields = fields;
    }
  },
}));
const auth = vi.hoisted(() => ({ user: null as unknown, endSession: vi.fn(), refresh: vi.fn() }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => auth }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));

import { ApiError } from '@/lib/api';
import { DeleteAccount } from '@/components/delete-account';
import { DeletionBanner } from '@/components/deletion-banner';

const me = (over: Partial<Me> = {}): Me => ({
  id: 'u1', email: 'owner@example.com', display_name: 'Owner', csrf_token: 'c', email_verified: true, verification_enforced: false,
  mail_delivery: 'smtp', deletion_scheduled_at: null, deletion_grace_days: 7, ...over,
});

beforeEach(() => {
  apiMock.account.requestDeletion.mockReset();
  apiMock.account.cancelDeletion.mockReset();
  apiMock.auth.signInProviders.mockReset();
  apiMock.auth.socialStartUrl.mockReset();
  apiMock.auth.socialStartUrl.mockImplementation((p: string, next: string | null) => `/api/v1/auth/oauth/${p}/start?next=${next}`);
  auth.endSession.mockReset();
  auth.refresh.mockReset();
  auth.user = me();
});

async function openDialog() {
  render(<DeleteAccount />);
  await userEvent.click(screen.getByRole('button', { name: /Delete account/ }));
}

describe('DeleteAccount', () => {
  it('says what happens and for how long before anything is asked', () => {
    auth.user = me({ deletion_grace_days: 14 });
    render(<DeleteAccount />);
    expect(screen.getByText(/It happens 14 days after you confirm/)).toBeInTheDocument();
    expect(apiMock.account.requestDeletion).not.toHaveBeenCalled();
  });

  it('keeps the button disabled until the email is typed exactly; the password is optional for sign-ups through Google or GitHub', async () => {
    await openDialog();
    const submit = screen.getByRole('button', { name: 'Delete my account' });
    expect(submit).toBeDisabled();
    expect(screen.getByText('Signed up with Google or GitHub? Leave this empty.')).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText(/Type owner@example.com to confirm/), 'owner@exampl');
    expect(submit).toBeDisabled();
    await userEvent.type(screen.getByLabelText(/Type owner@example.com to confirm/), 'e.com');
    expect(submit).toBeEnabled();
  });

  it('sends the request and ends the session with the reason, so the login page can explain', async () => {
    apiMock.account.requestDeletion.mockResolvedValue({ scheduled_for: '2026-10-16T12:00:00Z' });
    await openDialog();
    await userEvent.type(screen.getByLabelText('Your password'), 'secret-pw');
    await userEvent.type(screen.getByLabelText(/Type owner@example.com to confirm/), 'Owner@Example.com');
    await userEvent.click(screen.getByRole('button', { name: 'Delete my account' }));
    await waitFor(() => expect(auth.endSession).toHaveBeenCalledWith('deleted'));
    expect(apiMock.account.requestDeletion).toHaveBeenCalledWith('secret-pw', 'Owner@Example.com');
  });

  it('shows a wrong password next to the field and stays signed in', async () => {
    apiMock.account.requestDeletion.mockRejectedValue(new ApiError(400, 'VALIDATION_ERROR', 'the password is incorrect', null, { password: 'incorrect' }));
    await openDialog();
    await userEvent.type(screen.getByLabelText('Your password'), 'nope');
    await userEvent.type(screen.getByLabelText(/Type owner@example.com to confirm/), 'owner@example.com');
    await userEvent.click(screen.getByRole('button', { name: 'Delete my account' }));
    expect(await screen.findByText('That is not your current password.')).toBeInTheDocument();
    expect(auth.endSession).not.toHaveBeenCalled();
  });

  it('lets a password-less user delete with the email alone', async () => {
    apiMock.account.requestDeletion.mockResolvedValue({ scheduled_for: '2026-10-16T12:00:00Z' });
    await openDialog();
    await userEvent.type(screen.getByLabelText(/Type owner@example.com to confirm/), 'owner@example.com');
    await userEvent.click(screen.getByRole('button', { name: 'Delete my account' }));
    await waitFor(() => expect(auth.endSession).toHaveBeenCalledWith('deleted'));
    expect(apiMock.account.requestDeletion).toHaveBeenCalledWith('', 'owner@example.com');
  });

  it('offers "Sign in again with" each provider, returning to Settings, when the session is too old', async () => {
    apiMock.account.requestDeletion.mockRejectedValue(new ApiError(403, 'REAUTH_REQUIRED', 'sign in again to delete your account'));
    apiMock.auth.signInProviders.mockResolvedValue([{ id: 'github', name: 'GitHub' }, { id: 'google', name: 'Google' }]);
    await openDialog();
    expect(apiMock.auth.signInProviders).not.toHaveBeenCalled();
    await userEvent.type(screen.getByLabelText(/Type owner@example.com to confirm/), 'owner@example.com');
    await userEvent.click(screen.getByRole('button', { name: 'Delete my account' }));
    expect(await screen.findByText(/sign in again to delete your account/i)).toBeInTheDocument();
    const github = await screen.findByRole('link', { name: 'Sign in again with GitHub' });
    expect(github).toHaveAttribute('href', '/api/v1/auth/oauth/github/start?next=/settings');
    expect(screen.getByRole('link', { name: 'Sign in again with Google' })).toBeInTheDocument();
    expect(screen.queryByText('That is not your current password.')).toBeNull();
    expect(auth.endSession).not.toHaveBeenCalled();
  });

  it('shows any other failure and lets the user retry', async () => {
    apiMock.account.requestDeletion.mockRejectedValue(new ApiError(500, 'INTERNAL', ''));
    await openDialog();
    await userEvent.type(screen.getByLabelText('Your password'), 'pw');
    await userEvent.type(screen.getByLabelText(/Type owner@example.com to confirm/), 'owner@example.com');
    await userEvent.click(screen.getByRole('button', { name: 'Delete my account' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/had a problem/i);
    expect(screen.getByRole('button', { name: 'Delete my account' })).toBeEnabled();
  });
});

describe('DeletionBanner', () => {
  it('renders nothing for an account that is not scheduled', () => {
    const { container } = render(<DeletionBanner />);
    expect(container).toBeEmptyDOMElement();
  });

  it('shows the date and cancels the deletion, then re-reads the user', async () => {
    auth.user = me({ deletion_scheduled_at: '2026-10-16T12:00:00Z' });
    apiMock.account.cancelDeletion.mockResolvedValue(undefined);
    render(<DeletionBanner />);
    expect(screen.getByRole('alert')).toHaveTextContent(/will be deleted on/);
    await userEvent.click(screen.getByRole('button', { name: 'Cancel deletion' }));
    await waitFor(() => expect(auth.refresh).toHaveBeenCalled());
    expect(apiMock.account.cancelDeletion).toHaveBeenCalled();
  });

  it('reports a failed cancel', async () => {
    auth.user = me({ deletion_scheduled_at: '2026-10-16T12:00:00Z' });
    apiMock.account.cancelDeletion.mockRejectedValue(new ApiError(409, 'CONFLICT', 'no deletion is scheduled for this account'));
    render(<DeletionBanner />);
    await userEvent.click(screen.getByRole('button', { name: 'Cancel deletion' }));
    expect(await screen.findByText('no deletion is scheduled for this account')).toBeInTheDocument();
    expect(auth.refresh).not.toHaveBeenCalled();
  });
});

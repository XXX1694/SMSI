import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ToastProvider } from '@/components/toast';

const apiMock = vi.hoisted(() => ({ auth: { resendVerification: vi.fn(), changePassword: vi.fn() } }));
const authMock = vi.hoisted(() => ({ user: null as unknown }));
vi.mock('@/lib/api', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    status: number;
    code: string;
    requestId: string | null = null;
    constructor(status: number, code: string, message: string) {
      super(message);
      this.status = status;
      this.code = code;
    }
  },
}));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => authMock }));

import { EmailBanner } from '@/components/email-banner';
import { PasswordForm } from '@/components/password-form';
import { ApiError } from '@/lib/api';

const me = (over: Record<string, unknown> = {}) => ({
  id: 'u1', email: 'ann@example.com', display_name: 'Ann', csrf_token: 'c',
  email_verified: false, verification_enforced: true, mail_delivery: 'smtp', ...over,
});

beforeEach(() => {
  window.localStorage.clear();
  apiMock.auth.resendVerification.mockReset();
  apiMock.auth.changePassword.mockReset();
  authMock.user = me();
});

describe('EmailBanner', () => {
  it('asks an unverified user to verify and offers a resend', async () => {
    apiMock.auth.resendVerification.mockResolvedValue(undefined);
    render(<EmailBanner />);
    expect(screen.getByRole('alert')).toHaveTextContent('Verify your email');
    expect(screen.getByRole('alert')).toHaveTextContent('ann@example.com');
    await userEvent.click(screen.getByRole('button', { name: 'Resend email' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Link sent');
    expect(apiMock.auth.resendVerification).toHaveBeenCalledTimes(1);
  });

  it('shows the server message when resending fails (cooldown, outage)', async () => {
    apiMock.auth.resendVerification.mockRejectedValue(new ApiError(429, 'RATE_LIMITED', 'wait a minute and try again'));
    render(<EmailBanner />);
    await userEvent.click(screen.getByRole('button', { name: 'Resend email' }));
    expect(await screen.findByText('wait a minute and try again')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Resend email' })).toBeEnabled();
  });

  it('shows nothing for a verified user or when verification is not enforced', () => {
    authMock.user = me({ email_verified: true });
    const a = render(<EmailBanner />);
    expect(a.container).toBeEmptyDOMElement();
    a.unmount();
    authMock.user = me({ verification_enforced: false });
    const b = render(<EmailBanner />);
    expect(b.container).toBeEmptyDOMElement();
  });

  it('says honestly that email delivery is off, and remembers the dismissal', async () => {
    authMock.user = me({ verification_enforced: false, mail_delivery: 'log' });
    const first = render(<EmailBanner />);
    expect(screen.getByRole('note')).toHaveTextContent('Email delivery is not configured on this server');
    await userEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(first.container).toBeEmptyDOMElement();
    first.unmount();
    const again = render(<EmailBanner />);
    expect(again.container).toBeEmptyDOMElement();
  });

  it('renders nothing while signed out', () => {
    authMock.user = null;
    expect(render(<EmailBanner />).container).toBeEmptyDOMElement();
  });
});

describe('PasswordForm', () => {
  const setup = () => render(<ToastProvider><PasswordForm /></ToastProvider>);
  const fill = async (cur: string, next: string, again: string) => {
    await userEvent.type(screen.getByLabelText('Current password'), cur);
    await userEvent.type(screen.getByLabelText('New password'), next);
    await userEvent.type(screen.getByLabelText('Repeat the new password'), again);
    await userEvent.click(screen.getByRole('button', { name: 'Change password' }));
  };

  it('changes the password and clears the fields', async () => {
    apiMock.auth.changePassword.mockResolvedValue(undefined);
    setup();
    await fill('old password', 'a brand new password', 'a brand new password');
    expect(apiMock.auth.changePassword).toHaveBeenCalledWith('old password', 'a brand new password', false);
    expect(await screen.findByRole('status')).toHaveTextContent('API keys and MCP connections were not revoked');
    expect(screen.getByLabelText('Current password')).toHaveValue('');
  });

  it('revokes keys and connections only when the box is ticked', async () => {
    apiMock.auth.changePassword.mockResolvedValue(undefined);
    setup();
    await userEvent.click(screen.getByRole('checkbox', { name: 'Also revoke all API keys and MCP connections' }));
    await fill('old password', 'a brand new password', 'a brand new password');
    expect(apiMock.auth.changePassword).toHaveBeenCalledWith('old password', 'a brand new password', true);
    expect(await screen.findByRole('status')).toHaveTextContent('API keys and MCP connections were revoked');
  });

  it('shows a wrong current password from the server', async () => {
    apiMock.auth.changePassword.mockRejectedValue(new ApiError(400, 'VALIDATION_ERROR', 'current password is incorrect'));
    setup();
    await fill('nope', 'a brand new password', 'a brand new password');
    expect(await screen.findByRole('alert')).toHaveTextContent('current password is incorrect');
  });

  it('checks the new password locally', async () => {
    setup();
    await fill('old password', 'short', 'short');
    expect(screen.getByRole('alert')).toHaveTextContent('8 to 128');
    expect(apiMock.auth.changePassword).not.toHaveBeenCalled();
  });
});

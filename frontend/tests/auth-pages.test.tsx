import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({
  auth: { verifyEmail: vi.fn(), forgotPassword: vi.fn(), resetPassword: vi.fn() },
}));
const authMock = vi.hoisted(() => ({ user: null as unknown, refresh: vi.fn() }));
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

import { ForgotPasswordView } from '@/components/forgot-password-view';
import { ResetPasswordView } from '@/components/reset-password-view';
import { VerifyEmailView } from '@/components/verify-email-view';
import { ApiError } from '@/lib/api';
import { forgetHashToken } from '@/lib/hash-token';

const open = (path: string) => window.history.replaceState(null, '', path);
const badLink = () => new ApiError(400, 'VALIDATION_ERROR', 'link is invalid or has expired');

beforeEach(() => {
  forgetHashToken();
  open('/');
  Object.values(apiMock.auth).forEach((f) => f.mockReset());
  authMock.user = null;
  authMock.refresh.mockReset();
  authMock.refresh.mockResolvedValue(undefined);
});

describe('VerifyEmailView', () => {
  it('shows a loading state, then success, and clears the token from the address bar', async () => {
    let finish: () => void = () => undefined;
    apiMock.auth.verifyEmail.mockReturnValue(new Promise<void>((r) => (finish = r)));
    open('/verify-email#token=abc_DEF-123');
    render(<VerifyEmailView />);
    expect(screen.getByRole('status')).toHaveTextContent('One moment');
    expect(apiMock.auth.verifyEmail).toHaveBeenCalledWith('abc_DEF-123');
    expect(window.location.hash).toBe('');
    expect(window.location.href).not.toContain('abc_DEF-123');
    await act(async () => finish());
    expect(await screen.findByRole('heading', { name: 'Email verified' })).toBeInTheDocument();
    expect(authMock.refresh).toHaveBeenCalled();
    expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/login');
  });

  it('sends a signed-in user to the dashboard', async () => {
    authMock.user = { email: 'a@example.com' };
    apiMock.auth.verifyEmail.mockResolvedValue(undefined);
    open('/verify-email#token=t');
    render(<VerifyEmailView />);
    expect(await screen.findByRole('link', { name: 'Go to the dashboard' })).toHaveAttribute('href', '/dashboard');
  });

  it('explains an invalid, used or expired link', async () => {
    apiMock.auth.verifyEmail.mockRejectedValue(badLink());
    open('/verify-email#token=old');
    render(<VerifyEmailView />);
    expect(await screen.findByRole('heading', { name: 'This link no longer works' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /request a new link/i })).toHaveAttribute('href', '/login');
  });

  it('treats a missing token as an invalid link without calling the API', async () => {
    open('/verify-email');
    render(<VerifyEmailView />);
    expect(await screen.findByRole('heading', { name: 'This link no longer works' })).toBeInTheDocument();
    expect(apiMock.auth.verifyEmail).not.toHaveBeenCalled();
  });

  it('shows a retryable error for network or server failures, keeping the token', async () => {
    apiMock.auth.verifyEmail.mockRejectedValueOnce(new ApiError(0, 'NETWORK', 'Cannot reach the server.')).mockResolvedValueOnce(undefined);
    open('/verify-email#token=keepme');
    render(<VerifyEmailView />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Cannot reach the server.');
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('heading', { name: 'Email verified' })).toBeInTheDocument();
    expect(apiMock.auth.verifyEmail).toHaveBeenNthCalledWith(2, 'keepme');
  });
});

describe('ForgotPasswordView', () => {
  it('asks for an email and answers the same way for any address', async () => {
    apiMock.auth.forgotPassword.mockResolvedValue({ delivery: 'smtp' });
    render(<ForgotPasswordView />);
    const submit = screen.getByRole('button', { name: 'Send reset link' });
    expect(submit).toBeDisabled();
    await userEvent.type(screen.getByLabelText('Email'), '  who@example.com ');
    await userEvent.click(submit);
    expect(apiMock.auth.forgotPassword).toHaveBeenCalledWith('who@example.com');
    expect(await screen.findByRole('heading', { name: 'Check your inbox' })).toBeInTheDocument();
    expect(screen.getByText(/If an account exists for that address/)).toBeInTheDocument();
    expect(screen.queryByText(/not set up on this server/)).not.toBeInTheDocument();
  });

  it('says so when the server cannot deliver mail', async () => {
    apiMock.auth.forgotPassword.mockResolvedValue({ delivery: 'log' });
    render(<ForgotPasswordView />);
    await userEvent.type(screen.getByLabelText('Email'), 'a@example.com');
    await userEvent.click(screen.getByRole('button', { name: 'Send reset link' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Email delivery is not set up on this server');
  });

  it('shows the error and keeps the form when the request fails', async () => {
    apiMock.auth.forgotPassword.mockRejectedValue(new ApiError(429, 'RATE_LIMITED', 'too many requests'));
    render(<ForgotPasswordView />);
    await userEvent.type(screen.getByLabelText('Email'), 'a@example.com');
    await userEvent.click(screen.getByRole('button', { name: 'Send reset link' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('too many requests');
    expect(screen.getByLabelText('Email')).toHaveValue('a@example.com');
  });
});

describe('ResetPasswordView', () => {
  const fill = async (a: string, b: string) => {
    await userEvent.type(screen.getByLabelText('New password'), a);
    await userEvent.type(screen.getByLabelText('Repeat the new password'), b);
    await userEvent.click(screen.getByRole('button', { name: 'Save password' }));
  };

  it('reads the token from the fragment, clears it, and resets', async () => {
    apiMock.auth.resetPassword.mockResolvedValue(undefined);
    open('/reset-password#token=tok_1');
    render(<ResetPasswordView />);
    expect(window.location.hash).toBe('');
    await fill('a brand new password', 'a brand new password');
    expect(apiMock.auth.resetPassword).toHaveBeenCalledWith('tok_1', 'a brand new password');
    expect(await screen.findByRole('heading', { name: 'Password updated' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/login');
  });

  it('validates length and confirmation locally without spending the link', async () => {
    open('/reset-password#token=tok_1');
    render(<ResetPasswordView />);
    await fill('short', 'short');
    expect(screen.getByRole('alert')).toHaveTextContent('8 to 128');
    await userEvent.clear(screen.getByLabelText('New password'));
    await userEvent.clear(screen.getByLabelText('Repeat the new password'));
    await fill('a brand new password', 'something else entirely');
    expect(screen.getByRole('alert')).toHaveTextContent('do not match');
    expect(apiMock.auth.resetPassword).not.toHaveBeenCalled();
  });

  it('shows the invalid-link state for a missing token and for a rejected one', async () => {
    open('/reset-password');
    const first = render(<ResetPasswordView />);
    expect(await screen.findByRole('heading', { name: 'This link no longer works' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Request a new link' })).toHaveAttribute('href', '/forgot-password');
    first.unmount();

    forgetHashToken();
    apiMock.auth.resetPassword.mockRejectedValue(badLink());
    open('/reset-password#token=spent');
    render(<ResetPasswordView />);
    await fill('a brand new password', 'a brand new password');
    expect(await screen.findByRole('heading', { name: 'This link no longer works' })).toBeInTheDocument();
  });

  it('keeps the form and shows the message on a server error', async () => {
    apiMock.auth.resetPassword.mockRejectedValue(new ApiError(500, 'INTERNAL', 'internal server error'));
    open('/reset-password#token=tok_1');
    render(<ResetPasswordView />);
    await fill('a brand new password', 'a brand new password');
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('internal server error'));
    expect(screen.getByRole('button', { name: 'Save password' })).toBeInTheDocument();
  });
});

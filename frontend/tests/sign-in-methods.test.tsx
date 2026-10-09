import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({
  auth: { signInMethods: vi.fn(), linkIdentity: vi.fn(), unlinkIdentity: vi.fn(), setPassword: vi.fn(), signInProviders: vi.fn(), socialStartUrl: vi.fn() },
}));
vi.mock('next/navigation', () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
const auth = vi.hoisted(() => ({ user: { login_methods: ['password'] } as unknown, refresh: vi.fn() }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => auth }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock('@/components/toast', () => ({ useToast: () => toast }));
const nav = vi.hoisted(() => ({ navigateTo: vi.fn() }));
vi.mock('@/lib/navigate', () => nav);

import { ApiError } from '@/lib/api';
import { SignInMethods } from '@/components/sign-in-methods';

const PROVIDERS = [{ id: 'github', name: 'GitHub' }, { id: 'google', name: 'Google' }];
const github = { provider: 'github', email: 'sam@example.com', linked_at: '2026-10-01T09:00:00Z', last_login_at: null };

beforeEach(() => {
  for (const f of Object.values(apiMock.auth)) f.mockReset();
  apiMock.auth.signInProviders.mockResolvedValue(PROVIDERS);
  apiMock.auth.socialStartUrl.mockImplementation((p: string, next: string | null) => `/api/v1/auth/oauth/${p}/start?next=${next}`);
  apiMock.auth.signInMethods.mockResolvedValue({ identities: [], has_password: true });
  auth.user = { login_methods: ['password'] };
  auth.refresh.mockReset();
  toast.success.mockReset();
  nav.navigateTo.mockReset();
});

describe('SignInMethods list', () => {
  it('shows a loading state, then the password and each enabled provider', async () => {
    render(<SignInMethods />);
    expect(screen.getByRole('status')).toHaveTextContent('Loading');
    expect(await screen.findByText('You can sign in with your email and password.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect GitHub' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect Google' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Set a password' })).toBeNull();
  });

  it('shows the email and date of a connected provider with Disconnect', async () => {
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [github], has_password: true });
    render(<SignInMethods />);
    expect(await screen.findByText(/sam@example.com · connected/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect GitHub' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect Google' })).toBeInTheDocument();
  });

  it('says so when the list cannot be loaded, and retries', async () => {
    apiMock.auth.signInMethods.mockRejectedValueOnce(new ApiError(500, 'INTERNAL', ''));
    render(<SignInMethods />);
    expect(await screen.findByText('Could not load your sign-in methods')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('You can sign in with your email and password.')).toBeInTheDocument();
  });

  it('explains an empty server: no providers and none connected', async () => {
    apiMock.auth.signInProviders.mockResolvedValue([]);
    render(<SignInMethods />);
    expect(await screen.findByText(/No sign-in providers are set up on this server/)).toBeInTheDocument();
  });

  it('keeps linked providers visible when the provider list fails, and offers a retry for the rest', async () => {
    apiMock.auth.signInProviders.mockRejectedValue(new ApiError(500, 'INTERNAL', ''));
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [github], has_password: true });
    render(<SignInMethods />);
    expect(await screen.findByText('Could not load the providers you can connect.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect GitHub' })).toBeInTheDocument();
    expect(screen.queryByText(/switched off/)).toBeNull();
  });

  it('lists a provider the server switched off with Disconnect only', async () => {
    apiMock.auth.signInProviders.mockResolvedValue([{ id: 'google', name: 'Google' }]);
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [github], has_password: true });
    render(<SignInMethods />);
    expect(await screen.findByText(/GitHub sign-in is switched off on this server/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Disconnect GitHub' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Connect GitHub' })).toBeNull();
  });
});

describe('connect', () => {
  it('asks for the current password when there is one, then goes to the provider', async () => {
    apiMock.auth.linkIdentity.mockResolvedValue({ authorize_url: 'https://github.example/authorize?x=1' });
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Connect GitHub' }));
    const dialog = await screen.findByRole('dialog');
    const go = within(dialog).getByRole('button', { name: 'Continue to GitHub' });
    expect(go).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText('Your password'), 'secret-pw');
    await userEvent.click(go);
    await waitFor(() => expect(nav.navigateTo).toHaveBeenCalledWith('https://github.example/authorize?x=1'));
    expect(apiMock.auth.linkIdentity).toHaveBeenCalledWith('github', 'secret-pw');
  });

  it('asks for no password from an account without one', async () => {
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [github], has_password: false });
    apiMock.auth.linkIdentity.mockResolvedValue({ authorize_url: 'https://google.example/a' });
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Connect Google' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).queryByLabelText('Your password')).toBeNull();
    await userEvent.click(within(dialog).getByRole('button', { name: 'Continue to Google' }));
    await waitFor(() => expect(nav.navigateTo).toHaveBeenCalledWith('https://google.example/a'));
    expect(apiMock.auth.linkIdentity).toHaveBeenCalledWith('google', undefined);
  });

  it('shows a wrong password at the field and does not leave', async () => {
    apiMock.auth.linkIdentity.mockRejectedValue(new ApiError(400, 'VALIDATION_ERROR', 'x', null, { current_password: 'incorrect' }));
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Connect GitHub' }));
    const dialog = await screen.findByRole('dialog');
    await userEvent.type(within(dialog).getByLabelText('Your password'), 'nope');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Continue to GitHub' }));
    expect(await within(dialog).findByText('That is not your current password.')).toBeInTheDocument();
    expect(nav.navigateTo).not.toHaveBeenCalled();
  });

  it('offers sign-in-again buttons for the linked providers when the session is too old', async () => {
    auth.user = { login_methods: ['github'] };
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [github], has_password: false });
    apiMock.auth.linkIdentity.mockRejectedValue(new ApiError(403, 'REAUTH_REQUIRED', 'sign in again'));
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Connect Google' }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Continue to Google' }));
    const again = await screen.findByRole('link', { name: 'Sign in again with GitHub' });
    expect(again).toHaveAttribute('href', '/api/v1/auth/oauth/github/start?next=/settings');
    expect(screen.queryByRole('link', { name: 'Sign in again with Google' })).toBeNull();
    expect(nav.navigateTo).not.toHaveBeenCalled();
  });
});

describe('connect failures', () => {
  async function tryConnect() {
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Connect GitHub' }));
    const dialog = await screen.findByRole('dialog');
    await userEvent.type(within(dialog).getByLabelText('Your password'), 'pw');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Continue to GitHub' }));
    return dialog;
  }

  it('names a provider failure, not a network one, when the API gives no usable address', async () => {
    apiMock.auth.linkIdentity.mockResolvedValue({ authorize_url: '' });
    const dialog = await tryConnect();
    expect(await within(dialog).findByText(/GitHub reported a problem/)).toBeInTheDocument();
    expect(dialog).not.toHaveTextContent('rejected the post');
    expect(nav.navigateTo).not.toHaveBeenCalled();
  });

  it('explains 409: already connected or waiting for deletion', async () => {
    apiMock.auth.linkIdentity.mockRejectedValue(new ApiError(409, 'CONFLICT', 'x'));
    expect(await within(await tryConnect()).findByText(/already connected, or this account is waiting to be deleted/)).toBeInTheDocument();
    expect(nav.navigateTo).not.toHaveBeenCalled();
  });

  it('explains 404: the admin switched the provider off meanwhile', async () => {
    apiMock.auth.linkIdentity.mockRejectedValue(new ApiError(404, 'NOT_FOUND', 'x'));
    expect(await within(await tryConnect()).findByText(/does not offer that provider any more/)).toBeInTheDocument();
  });

  it('answers an unknown error code with the generic sentence, never the code', async () => {
    apiMock.auth.linkIdentity.mockRejectedValue(new ApiError(418, 'TEAPOT_BREWING', 'TEAPOT_BREWING'));
    const dialog = await tryConnect();
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Something went wrong. Try again.');
    expect(dialog).not.toHaveTextContent('TEAPOT');
  });

  it('says so when the user is linked only to providers the server no longer offers', async () => {
    auth.user = { login_methods: ['github'] };
    apiMock.auth.signInProviders.mockResolvedValue([{ id: 'google', name: 'Google' }]);
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [github], has_password: false });
    apiMock.auth.linkIdentity.mockRejectedValue(new ApiError(403, 'REAUTH_REQUIRED', 'x'));
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Connect Google' }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Continue to Google' }));
    expect(await screen.findByText(/None of the providers you signed up with is available/)).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: /Sign in again with/ })).toBeNull();
  });
});

describe('disconnect', () => {
  beforeEach(() => {
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [github], has_password: true });
  });

  it('confirms with the password, refreshes the list and the user, and says so', async () => {
    apiMock.auth.unlinkIdentity.mockResolvedValue(undefined);
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Disconnect GitHub' }));
    const dialog = await screen.findByRole('dialog');
    await userEvent.type(within(dialog).getByLabelText('Your password'), 'secret-pw');
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [], has_password: true });
    await userEvent.click(within(dialog).getByRole('button', { name: 'Disconnect' }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('GitHub disconnected.'));
    expect(apiMock.auth.unlinkIdentity).toHaveBeenCalledWith('github', 'secret-pw');
    expect(auth.refresh).toHaveBeenCalled();
    expect(await screen.findByRole('button', { name: 'Connect GitHub' })).toBeInTheDocument();
  });

  it('explains the last method: 409 says to keep one way to sign in', async () => {
    apiMock.auth.unlinkIdentity.mockRejectedValue(new ApiError(409, 'CONFLICT', 'keep at least one way to sign in'));
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Disconnect GitHub' }));
    const dialog = await screen.findByRole('dialog');
    await userEvent.type(within(dialog).getByLabelText('Your password'), 'pw');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Disconnect' }));
    expect(await within(dialog).findByText(/Keep at least one way to sign in/)).toBeInTheDocument();
    expect(toast.success).not.toHaveBeenCalled();
  });
});

describe('set a password', () => {
  beforeEach(() => {
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [github], has_password: false });
  });

  it('is offered only to accounts without one, validates 8 to 128 characters, then reloads', async () => {
    apiMock.auth.setPassword.mockResolvedValue(undefined);
    render(<SignInMethods />);
    expect(await screen.findByText(/You have no password/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Set a password' }));
    await userEvent.type(screen.getByLabelText('New password'), 'short');
    await userEvent.click(screen.getByRole('button', { name: 'Set password' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Use 8–128 characters.');
    expect(apiMock.auth.setPassword).not.toHaveBeenCalled();
    await userEvent.type(screen.getByLabelText('New password'), '-and-longer');
    apiMock.auth.signInMethods.mockResolvedValue({ identities: [github], has_password: true });
    await userEvent.click(screen.getByRole('button', { name: 'Set password' }));
    await waitFor(() => expect(apiMock.auth.setPassword).toHaveBeenCalledWith('short-and-longer'));
    expect(toast.success).toHaveBeenCalledWith('Password set. Your other sessions were signed out.');
    expect(auth.refresh).toHaveBeenCalled();
    expect(await screen.findByText('You can sign in with your email and password.')).toBeInTheDocument();
  });

  async function submitPassword() {
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Set a password' }));
    await userEvent.type(screen.getByLabelText('New password'), 'long-enough-pw');
    await userEvent.click(screen.getByRole('button', { name: 'Set password' }));
  }

  it('keeps the form and shows the failure on a 400', async () => {
    apiMock.auth.setPassword.mockRejectedValue(new ApiError(400, 'VALIDATION_ERROR', 'password must be 8-128 characters', null, { new_password: 'bad' }));
    await submitPassword();
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Set password' })).toBeEnabled();
    expect(toast.success).not.toHaveBeenCalled();
  });

  it('re-reads the account on a 409 (a password was set elsewhere) instead of showing an error', async () => {
    apiMock.auth.setPassword.mockRejectedValue(new ApiError(409, 'CONFLICT', 'you already have a password'));
    apiMock.auth.signInMethods.mockResolvedValueOnce({ identities: [github], has_password: false }).mockResolvedValue({ identities: [github], has_password: true });
    await submitPassword();
    expect(await screen.findByText('You can sign in with your email and password.')).toBeInTheDocument();
    expect(auth.refresh).toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
  });

  it('shows the sign-in-again notice when the session is too old', async () => {
    auth.user = { login_methods: ['github'] };
    apiMock.auth.setPassword.mockRejectedValue(new ApiError(403, 'REAUTH_REQUIRED', 'sign in again'));
    render(<SignInMethods />);
    await userEvent.click(await screen.findByRole('button', { name: 'Set a password' }));
    await userEvent.type(screen.getByLabelText('New password'), 'long-enough-pw');
    await userEvent.click(screen.getByRole('button', { name: 'Set password' }));
    expect(await screen.findByRole('link', { name: 'Sign in again with GitHub' })).toBeInTheDocument();
  });
});

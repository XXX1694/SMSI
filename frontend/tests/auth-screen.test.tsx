import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const auth = vi.hoisted(() => ({ login: vi.fn(), register: vi.fn() }));
const router = vi.hoisted(() => ({ replace: vi.fn(), push: vi.fn() }));
const query = vi.hoisted(() => ({ value: '' }));
const apiMock = vi.hoisted(() => ({ signInProviders: vi.fn(), socialStartUrl: undefined as unknown }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => auth }));
vi.mock('next/navigation', () => ({ useRouter: () => router, useSearchParams: () => new URLSearchParams(query.value) }));
vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api');
  apiMock.socialStartUrl = actual.api.auth.socialStartUrl;
  return { ...actual, api: { auth: apiMock } };
});

import { AuthForm } from '@/components/auth-form';
import { ApiError } from '@/lib/api';

const BOTH = [
  { id: 'github', name: 'GitHub' },
  { id: 'google', name: 'Google' },
];

beforeEach(() => {
  query.value = '';
  apiMock.signInProviders.mockReset();
  apiMock.signInProviders.mockResolvedValue(BOTH);
  auth.login.mockReset().mockResolvedValue(undefined);
  auth.register.mockReset().mockResolvedValue(undefined);
  router.replace.mockReset();
});

const open = async (mode: 'login' | 'register') => {
  render(<AuthForm mode={mode} />);
  await waitFor(() => expect(screen.queryByText('Loading sign-in options…')).toBeNull());
};

describe('provider buttons', () => {
  it('shows one button per provider the server offers, above the email form', async () => {
    await open('login');
    const github = screen.getByRole('link', { name: 'Continue with GitHub' });
    expect(github).toHaveAttribute('href', '/api/v1/auth/oauth/github/start');
    expect(screen.getByRole('link', { name: 'Continue with Google' })).toHaveAttribute('href', '/api/v1/auth/oauth/google/start');
    expect(screen.getByRole('separator')).toBeInTheDocument();
    expect(github.compareDocumentPosition(screen.getByLabelText('Email')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('shows only the providers in the list', async () => {
    apiMock.signInProviders.mockResolvedValue([{ id: 'github', name: 'GitHub' }]);
    await open('register');
    expect(screen.getByRole('link', { name: 'Continue with GitHub' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: /Google/ })).toBeNull();
  });

  it('hides the block and the divider when no provider is configured', async () => {
    apiMock.signInProviders.mockResolvedValue([]);
    await open('login');
    expect(screen.queryByRole('link', { name: /Continue with/ })).toBeNull();
    expect(screen.queryByRole('separator')).toBeNull();
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
  });

  it('shows a loading placeholder first, with a text for screen readers', async () => {
    apiMock.signInProviders.mockReturnValue(new Promise(() => undefined));
    render(<AuthForm mode="login" />);
    expect(screen.getByRole('status')).toHaveTextContent('Loading sign-in options…');
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
  });

  it('says so when the list could not be loaded, keeps the email form and retries', async () => {
    apiMock.signInProviders.mockRejectedValueOnce(new ApiError(500, 'INTERNAL', '')).mockResolvedValue(BOTH);
    await open('login');
    expect(await screen.findByRole('alert')).toHaveTextContent('Sign-in options could not be loaded. You can still use your email.');
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('link', { name: 'Continue with GitHub' })).toBeInTheDocument();
  });

  it('carries a safe next into the provider start URL', async () => {
    query.value = 'next=%2Fcompose%3Fpost%3D1';
    await open('login');
    expect(screen.getByRole('link', { name: 'Continue with GitHub' })).toHaveAttribute('href', '/api/v1/auth/oauth/github/start?next=%2Fcompose%3Fpost%3D1');
  });
});

describe('next is kept', () => {
  it('on the Create account and Sign in links', async () => {
    query.value = 'next=%2Fdashboard%3Fa%3Db';
    await open('login');
    expect(screen.getByRole('link', { name: 'Create one' })).toHaveAttribute('href', '/register?next=%2Fdashboard%3Fa%3Db');
    document.body.innerHTML = '';
    await open('register');
    expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/login?next=%2Fdashboard%3Fa%3Db');
  });

  it('drops an off-site next everywhere', async () => {
    query.value = 'next=%2F%2Fevil.example';
    await open('login');
    expect(screen.getByRole('link', { name: 'Create one' })).toHaveAttribute('href', '/register');
    expect(screen.getByRole('link', { name: 'Continue with GitHub' })).toHaveAttribute('href', '/api/v1/auth/oauth/github/start');
    await userEvent.type(screen.getByLabelText('Email'), 'a@example.com');
    await userEvent.type(screen.getByLabelText('Password'), 'pw');
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith('/dashboard'));
  });

  it('after signing in', async () => {
    query.value = 'next=%2Fposts';
    await open('login');
    await userEvent.type(screen.getByLabelText('Email'), ' a@example.com ');
    await userEvent.type(screen.getByLabelText('Password'), 'pw');
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith('/posts'));
    expect(auth.login).toHaveBeenCalledWith('a@example.com', 'pw');
  });
});

describe('email form', () => {
  it('has the mobile-friendly attributes on the email input', async () => {
    await open('register');
    const email = screen.getByLabelText('Email');
    expect(email).toHaveAttribute('autocapitalize', 'none');
    expect(email).toHaveAttribute('spellcheck', 'false');
    expect(email).toHaveAttribute('autocomplete', 'email');
    expect(email).toHaveAttribute('inputmode', 'email');
    expect(screen.getByLabelText('Password')).toHaveAttribute('autocomplete', 'new-password');
  });

  it('asks for email and password only when registering (no name)', async () => {
    await open('register');
    expect(screen.queryByLabelText(/Name/)).toBeNull();
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
  });

  it('shows and hides the password', async () => {
    await open('login');
    const password = screen.getByLabelText('Password');
    expect(password).toHaveAttribute('type', 'password');
    const toggle = screen.getByRole('button', { name: 'Show Password' });
    expect(toggle).toHaveAttribute('aria-pressed', 'false');
    await userEvent.click(toggle);
    expect(password).toHaveAttribute('type', 'text');
    // The name stays fixed; aria-pressed carries the state.
    expect(screen.getByRole('button', { name: 'Show Password' })).toHaveAttribute('aria-pressed', 'true');
    await userEvent.click(toggle);
    expect(password).toHaveAttribute('type', 'password');
  });

  it('does not disable the submit button; an empty submit says what is missing and focuses the first field', async () => {
    await open('register');
    const submit = screen.getByRole('button', { name: 'Create account' });
    expect(submit).toBeEnabled();
    await userEvent.click(submit);
    expect(screen.getByText('Enter your email address.')).toBeInTheDocument();
    expect(screen.getByText('Enter your password.')).toBeInTheDocument();
    expect(screen.getByText('Accept the Terms and the Privacy Policy to create an account.')).toBeInTheDocument();
    expect(screen.getByText('Fix the highlighted fields to continue.')).toBeInTheDocument();
    expect(screen.getByLabelText('Email')).toHaveFocus();
    expect(screen.getByLabelText('Email')).toBeInvalid();
    expect(auth.register).not.toHaveBeenCalled();
  });

  it('does not complain about a field that was only tabbed through', async () => {
    const user = userEvent.setup();
    await open('register');
    await user.click(screen.getByLabelText('Email'));
    await user.tab();
    await user.tab();
    expect(screen.queryByText('Enter your email address.')).toBeNull();
    expect(screen.queryByText('Enter your password.')).toBeNull();
  });

  it('announces one summary alert on submit, not one per field', async () => {
    await open('register');
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }));
    const alerts = screen.getAllByRole('alert');
    expect(alerts).toHaveLength(1);
    expect(alerts[0]).toHaveTextContent('Fix the highlighted fields to continue.');
    expect(screen.getByLabelText('Email')).toHaveAccessibleDescription('Enter your email address.');
  });

  it('keeps the Terms box unchanged when a link in its text is used, and toggles it from the text', async () => {
    await open('register');
    const box = screen.getByRole('checkbox');
    const link = screen.getAllByRole('link', { name: 'Terms' })[0] as HTMLElement;
    link.addEventListener('click', (e) => e.preventDefault());
    await userEvent.click(link);
    expect(box).not.toBeChecked();
    await userEvent.click(screen.getByText(/I agree to the/));
    expect(box).toBeChecked();
  });

  it('checks a field when it loses focus, and clears the message once it is fixed', async () => {
    const user = userEvent.setup();
    await open('register');
    await user.type(screen.getByLabelText('Email'), 'not-an-email');
    await user.tab();
    expect(screen.getByText('Enter a valid email address, for example name@example.com.')).toBeInTheDocument();
    await user.clear(screen.getByLabelText('Email'));
    await user.type(screen.getByLabelText('Email'), 'sam@example.com');
    await user.tab();
    expect(screen.queryByText(/Enter a valid email/)).toBeNull();
    await user.type(screen.getByLabelText('Password'), 'short');
    await user.tab();
    expect(screen.getByText('Use 8–128 characters.')).toBeInTheDocument();
  });

  it('puts the fields the server refused on the fields', async () => {
    auth.register.mockRejectedValue(new ApiError(400, 'VALIDATION_ERROR', 'invalid email address', null, { email: 'invalid email', password: 'invalid length' }));
    const user = userEvent.setup();
    await open('register');
    await user.type(screen.getByLabelText('Email'), 'sam@example.com');
    await user.type(screen.getByLabelText('Password'), 'longenough1');
    await user.click(screen.getByRole('checkbox'));
    await user.click(screen.getByRole('button', { name: 'Create account' }));
    expect(await screen.findByText('Enter a valid email address, for example name@example.com.')).toBeInTheDocument();
    expect(screen.getByText('Use 8–128 characters.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled();
  });

  it('offers "Sign in" (keeping next) and "Forgot password?" when the email is taken', async () => {
    query.value = 'next=%2Fposts';
    auth.register.mockRejectedValue(new ApiError(409, 'CONFLICT', 'an account with this email already exists'));
    const user = userEvent.setup();
    await open('register');
    await user.type(screen.getByLabelText('Email'), 'sam@example.com');
    await user.type(screen.getByLabelText('Password'), 'longenough1');
    await user.click(screen.getByRole('checkbox'));
    await user.click(screen.getByRole('button', { name: 'Create account' }));
    const alert = await screen.findByText(/An account with this email already exists/);
    const box = alert.closest('[role="alert"]') as HTMLElement;
    expect(within(box).getByRole('link', { name: 'Sign in' })).toHaveAttribute('href', '/login?next=%2Fposts');
    expect(within(box).getByRole('link', { name: 'Forgot password?' })).toHaveAttribute('href', '/forgot-password');
  });

  it('shows any other failure above the button and keeps the form usable', async () => {
    auth.login.mockRejectedValue(new ApiError(401, 'UNAUTHENTICATED', 'Invalid email or password'));
    const user = userEvent.setup();
    await open('login');
    await user.type(screen.getByLabelText('Email'), 'sam@example.com');
    await user.type(screen.getByLabelText('Password'), 'wrong-password');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(await screen.findByText('Invalid email or password')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled();
  });
});

describe('/login?error=', () => {
  it.each([
    ['oauth_cancelled', /Sign-in was canceled/],
    ['oauth_state_invalid', /expired or was opened in another browser/],
    ['oauth_provider_error', /Sign-in did not finish because the provider reported a problem/],
    ['email_unverified', /Verify your primary email with your sign-in provider, then try again/],
    ['account_exists', /Sign in with your password, then connect your sign-in provider in Settings/],
    ['identity_in_use', /already connected to another Steerpost account/],
    ['account_unavailable', /cannot sign in right now/],
    ['signup_expired', /sign-up expired/],
    ['something_new', /Sign-in did not work/],
  ])('explains %s in plain words', async (code, text) => {
    query.value = `error=${code}`;
    await open('login');
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent(text);
    expect(alert).not.toHaveTextContent(code);
  });

  it('names the provider when the API says which one', async () => {
    query.value = 'error=email_unverified&provider=github';
    await open('login');
    expect(screen.getByRole('alert')).toHaveTextContent('Verify your primary email on GitHub, then try again.');
  });

  it('never echoes an unknown provider value', async () => {
    query.value = 'error=account_exists&provider=%3Cscript%3E';
    await open('login');
    expect(screen.getByRole('alert')).toHaveTextContent('connect your sign-in provider in Settings');
  });

  it('shows nothing on the register page or without an error', async () => {
    await open('login');
    expect(screen.queryByRole('alert')).toBeNull();
    document.body.innerHTML = '';
    query.value = 'error=account_exists';
    await open('register');
    expect(screen.queryByRole('alert')).toBeNull();
  });
});

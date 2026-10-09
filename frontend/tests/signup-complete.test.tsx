import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const auth = vi.hoisted(() => ({ completeSignup: vi.fn() }));
const router = vi.hoisted(() => ({ replace: vi.fn() }));
const apiMock = vi.hoisted(() => ({ pendingSignup: vi.fn() }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => auth }));
vi.mock('next/navigation', () => ({ useRouter: () => router }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: { auth: apiMock } }));

import { SignupCompleteView } from '@/components/signup-complete-view';
import { ApiError } from '@/lib/api';

const pending = { provider: 'github', email: 'sam@example.com', display_name: 'Sam Rivera', next: '/posts' };

beforeEach(() => {
  apiMock.pendingSignup.mockReset().mockResolvedValue(pending);
  auth.completeSignup.mockReset().mockResolvedValue(undefined);
  router.replace.mockReset();
});

const ready = async () => {
  render(<SignupCompleteView />);
  return screen.findByLabelText('Email');
};

describe('/signup/complete', () => {
  it('shows a loading state first', () => {
    apiMock.pendingSignup.mockReturnValue(new Promise(() => undefined));
    render(<SignupCompleteView />);
    expect(screen.getByRole('heading', { name: 'Finish creating your account' })).toBeInTheDocument();
    expect(screen.getByRole('status')).toBeInTheDocument();
  });

  it('shows the email read-only, the prefilled name, the Terms and the provider', async () => {
    const email = await ready();
    expect(email).toHaveValue('sam@example.com');
    expect(email).toHaveAttribute('readonly');
    expect(screen.getByText('Verified by GitHub. You cannot change it here.')).toBeInTheDocument();
    expect(screen.getByText('You signed in with GitHub. Accept the Terms to create your account.')).toBeInTheDocument();
    expect(screen.getByLabelText('Name (optional)')).toHaveValue('Sam Rivera');
    expect(screen.getByRole('checkbox')).not.toBeChecked();
    expect(screen.getAllByRole('link', { name: 'Terms' })[0]).toHaveAttribute('href', '/terms');
    expect(screen.getByRole('link', { name: 'Back to sign in' })).toHaveAttribute('href', '/login');
  });

  it('does not create the account until the Terms are accepted (D-016), and says why', async () => {
    await ready();
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }));
    expect(screen.getByText('Accept the Terms and the Privacy Policy to create an account.')).toBeInTheDocument();
    expect(screen.getByRole('checkbox')).toHaveFocus();
    expect(auth.completeSignup).not.toHaveBeenCalled();
  });

  it('creates the account with the edited name and goes to next', async () => {
    await ready();
    const name = screen.getByLabelText('Name (optional)');
    await userEvent.clear(name);
    await userEvent.type(name, 'Sam R.');
    await userEvent.click(screen.getByRole('checkbox'));
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }));
    await waitFor(() => expect(auth.completeSignup).toHaveBeenCalledWith('Sam R.', true));
    expect(router.replace).toHaveBeenCalledWith('/posts');
  });

  it('goes to the dashboard when next is missing or off-site', async () => {
    apiMock.pendingSignup.mockResolvedValue({ ...pending, next: '//evil.example' });
    await ready();
    await userEvent.click(screen.getByRole('checkbox'));
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith('/dashboard'));
  });

  it('sends the person back to /login with a message when there is no sign-up (404)', async () => {
    apiMock.pendingSignup.mockRejectedValue(new ApiError(404, 'NOT_FOUND', 'no sign-up is waiting'));
    render(<SignupCompleteView />);
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith('/login?error=signup_expired'));
  });

  it('shows a load failure that is not a 404 with Try again, and does not leave the page', async () => {
    apiMock.pendingSignup.mockRejectedValueOnce(new ApiError(500, 'INTERNAL', '')).mockResolvedValue(pending);
    render(<SignupCompleteView />);
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(router.replace).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByLabelText('Email')).toBeInTheDocument();
  });

  it('shows the error and stays when creating fails', async () => {
    auth.completeSignup.mockRejectedValue(new ApiError(500, 'INTERNAL', ''));
    await ready();
    await userEvent.click(screen.getByRole('checkbox'));
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/had a problem/i);
    expect(router.replace).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Create account' })).toBeEnabled();
  });

  it('puts a too-long name on the name field', async () => {
    auth.completeSignup.mockRejectedValue(new ApiError(400, 'VALIDATION_ERROR', 'display_name too long', null, { display_name: 'max 100 characters' }));
    await ready();
    await userEvent.click(screen.getByRole('checkbox'));
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }));
    expect(await screen.findByText('Use 100 characters or fewer.')).toBeInTheDocument();
  });

  it('goes back to /login when the ticket expired while the form was open', async () => {
    auth.completeSignup.mockRejectedValue(new ApiError(404, 'NOT_FOUND', 'no sign-up is waiting'));
    await ready();
    await userEvent.click(screen.getByRole('checkbox'));
    await userEvent.click(screen.getByRole('button', { name: 'Create account' }));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith('/login?error=signup_expired'));
  });
});

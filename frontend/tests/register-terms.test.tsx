import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const auth = vi.hoisted(() => ({ login: vi.fn(), register: vi.fn() }));
const apiMock = vi.hoisted(() => ({ auth: { signInProviders: vi.fn() } }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
const router = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => auth }));
vi.mock('next/navigation', () => ({ useRouter: () => router, useSearchParams: () => new URLSearchParams('') }));

import { AuthForm } from '@/components/auth-form';

beforeEach(() => {
  apiMock.auth.signInProviders.mockResolvedValue([]);
  auth.register.mockReset();
  auth.register.mockResolvedValue(undefined);
  router.replace.mockReset();
});

async function fill(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Email'), 'sam@example.com');
  await user.type(screen.getByLabelText('Password'), 'longenough1');
}

describe('register form terms', () => {
  it('links to the Terms and the Privacy Policy next to an unchecked box', () => {
    render(<AuthForm mode="register" />);
    expect(screen.getByRole('checkbox', { name: /I agree to the Terms and the Privacy Policy/ })).not.toBeChecked();
    expect(screen.getAllByRole('link', { name: 'Terms' })[0]).toHaveAttribute('href', '/terms');
    expect(screen.getAllByRole('link', { name: 'Privacy Policy' })[0]).toHaveAttribute('href', '/privacy');
  });

  it('does not register until the box is checked', async () => {
    const user = userEvent.setup();
    render(<AuthForm mode="register" />);
    await fill(user);
    await user.click(screen.getByRole('button', { name: 'Create account' }));
    expect(screen.getByText('Accept the Terms and the Privacy Policy to create an account.')).toBeInTheDocument();
    expect(auth.register).not.toHaveBeenCalled();
  });

  it('sends accept_terms through the provider once the box is checked', async () => {
    const user = userEvent.setup();
    render(<AuthForm mode="register" />);
    await fill(user);
    await user.click(screen.getByRole('checkbox'));
    await user.click(screen.getByRole('button', { name: 'Create account' }));
    await waitFor(() => expect(auth.register).toHaveBeenCalledWith('sam@example.com', 'longenough1', '', true));
    expect(router.replace).toHaveBeenCalledWith('/dashboard');
  });

  it('has no checkbox on the sign-in form but still links to both pages', () => {
    render(<AuthForm mode="login" />);
    expect(screen.queryByRole('checkbox')).toBeNull();
    expect(screen.getByRole('link', { name: 'Terms' })).toHaveAttribute('href', '/terms');
    expect(screen.getByRole('link', { name: 'Privacy' })).toHaveAttribute('href', '/privacy');
  });
});

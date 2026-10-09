import { render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const query = vi.hoisted(() => ({ value: '' }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ replace: vi.fn() }), useSearchParams: () => new URLSearchParams(query.value) }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => ({ login: vi.fn(), register: vi.fn() }) }));
vi.mock('@/lib/api', () => ({ api: { auth: { signInProviders: vi.fn().mockResolvedValue([]) } }, ApiError: class extends Error {} }));

import { AuthForm } from '@/components/auth-form';

beforeEach(() => {
  query.value = '';
});

describe('login after account deletion was requested', () => {
  it('explains that deletion is scheduled and how to cancel it', async () => {
    query.value = 'deleted=1';
    render(<AuthForm mode="login" />);
    await waitFor(() => expect(screen.queryByText('Loading sign-in options…')).toBeNull());
    expect(screen.getByRole('status')).toHaveTextContent(/Deletion of your account is scheduled/);
    expect(screen.getByRole('status')).toHaveTextContent(/cancel/);
  });

  it('shows nothing extra on a normal visit or on the register page', async () => {
    const first = render(<AuthForm mode="login" />);
    await waitFor(() => expect(screen.queryByText('Loading sign-in options…')).toBeNull());
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    first.unmount();
    query.value = 'deleted=1';
    render(<AuthForm mode="register" />);
    await waitFor(() => expect(screen.queryByText('Loading sign-in options…')).toBeNull());
    expect(screen.queryAllByRole('status')).toHaveLength(0);
  });
});

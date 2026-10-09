import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({ auth: { me: vi.fn(), completeSignup: vi.fn() } }));
const router = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('next/navigation', () => ({ useRouter: () => router, usePathname: () => '/posts' }));
vi.mock('@/components/app-shell', () => ({ AppShell: ({ children }: { children: React.ReactNode }) => <div>{children}</div> }));

import { AuthProvider, useAuth } from '@/components/auth-provider';
import { ApiError } from '@/lib/api';
import AppLayout from '@/app/(app)/layout';

const me = { id: 'u1', email: 'a@example.com', csrf_token: 't' };
const mount = () =>
  render(
    <AuthProvider>
      <AppLayout>
        <p>page body</p>
      </AppLayout>
    </AuthProvider>,
  );

beforeEach(() => {
  apiMock.auth.me.mockReset();
  apiMock.auth.completeSignup.mockReset();
  router.replace.mockReset();
});

describe('AuthProvider with /me', () => {
  it('signs the user out only on 401', async () => {
    apiMock.auth.me.mockRejectedValue(new ApiError(401, 'UNAUTHENTICATED', 'Please sign in.'));
    mount();
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith('/login?next=%2Fposts'));
  });

  it.each([
    ['a server error', new ApiError(500, 'INTERNAL', 'Request failed (500).')],
    ['a network failure', new ApiError(0, 'NETWORK', 'Cannot reach the server. Check your connection.')],
  ])('shows an error with Retry on %s instead of logging the user out', async (_n, err) => {
    apiMock.auth.me.mockRejectedValueOnce(err).mockResolvedValue(me);
    mount();
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(router.replace).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('page body')).toBeInTheDocument();
    expect(apiMock.auth.me).toHaveBeenCalledTimes(2);
  });
});

describe('AuthProvider.completeSignup', () => {
  function Probe() {
    const { user, completeSignup } = useAuth();
    return (
      <>
        <p>{user ? `in as ${user.email}` : 'signed out'}</p>
        <button onClick={() => void completeSignup('Ann', true)}>finish</button>
      </>
    );
  }

  it('signs the new user in with what the API returned', async () => {
    apiMock.auth.me.mockRejectedValue(new ApiError(401, 'UNAUTHENTICATED', 'Please sign in.'));
    apiMock.auth.completeSignup.mockResolvedValue({ ...me, csrf_token: 'fresh' });
    render(
      <AuthProvider>
        <Probe />
      </AuthProvider>,
    );
    expect(await screen.findByText('signed out')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'finish' }));
    expect(await screen.findByText('in as a@example.com')).toBeInTheDocument();
    expect(apiMock.auth.completeSignup).toHaveBeenCalledWith({ display_name: 'Ann', accept_terms: true });
  });
});

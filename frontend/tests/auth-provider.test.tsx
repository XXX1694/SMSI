import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({ auth: { me: vi.fn() } }));
const router = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('next/navigation', () => ({ useRouter: () => router, usePathname: () => '/posts' }));
vi.mock('@/components/app-shell', () => ({ AppShell: ({ children }: { children: React.ReactNode }) => <div>{children}</div> }));

import { AuthProvider } from '@/components/auth-provider';
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

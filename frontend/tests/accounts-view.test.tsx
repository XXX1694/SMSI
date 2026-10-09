import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({ social: { providers: vi.fn(), accounts: vi.fn(), connectUrl: vi.fn(() => '/x'), disconnect: vi.fn() } }));
const router = vi.hoisted(() => ({ replace: vi.fn() }));
const search = vi.hoisted(() => ({ value: '' }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
vi.mock('@/components/toast', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn() }) }));
vi.mock('next/navigation', () => ({ useRouter: () => router, usePathname: () => '/accounts', useSearchParams: () => new URLSearchParams(search.value) }));

import { AccountsView } from '@/components/accounts-view';

beforeEach(() => {
  apiMock.social.providers.mockResolvedValue([]);
  apiMock.social.accounts.mockResolvedValue([]);
  router.replace.mockReset();
});

describe('AccountsView connect result', () => {
  it('explains a failed connection in plain words and clears the query parameters', async () => {
    search.value = 'error=access_denied&provider=linkedin';
    render(<AccountsView />);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('cancelled');
    expect(alert).not.toHaveTextContent('access_denied');
    expect(router.replace).toHaveBeenCalledWith('/accounts');
  });

  it('confirms a successful connection once and clears the query parameters', async () => {
    search.value = 'connected=linkedin';
    render(<AccountsView />);
    expect(await screen.findByText(/Connected linkedin successfully/i)).toBeInTheDocument();
    expect(router.replace).toHaveBeenCalledWith('/accounts');
  });
});

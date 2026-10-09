import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMock = vi.hoisted(() => ({ social: { providers: vi.fn(), accounts: vi.fn(), connectUrl: vi.fn(() => '/x'), disconnect: vi.fn(), connectWithToken: vi.fn() } }));
const router = vi.hoisted(() => ({ replace: vi.fn() }));
const search = vi.hoisted(() => ({ value: '' }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock('@/components/toast', () => ({ useToast: () => toast }));
vi.mock('next/navigation', () => ({ useRouter: () => router, usePathname: () => '/accounts', useSearchParams: () => new URLSearchParams(search.value) }));

import { AccountsView } from '@/components/accounts-view';
import { normalizeProvider } from '@/lib/normalize';

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
    expect(alert).toHaveTextContent('canceled');
    expect(alert).not.toHaveTextContent('access_denied');
    expect(router.replace).toHaveBeenCalledWith('/accounts');
  });

  it('confirms a successful connection once and clears the query parameters', async () => {
    search.value = 'connected=linkedin';
    render(<AccountsView />);
    expect(await screen.findByText(/LinkedIn connected\./i)).toBeInTheDocument();
    expect(router.replace).toHaveBeenCalledWith('/accounts');
  });
});

describe('AccountsView connect with a token', () => {
  const discord = normalizeProvider({
    provider: 'discord',
    configured: true,
    status: 'supported',
    capabilities: {
      can_publish_text: true,
      connect_method: 'token',
      connect_fields: [{ name: 'webhook_url', label: 'Webhook URL', kind: 'url', secret: true, required: true }],
    },
  });

  it('opens the form, connects, refreshes the list and confirms', async () => {
    search.value = '';
    apiMock.social.accounts.mockClear();
    const account = { id: 'a1', provider: 'discord', username: '#general', display_name: 'Discord #general', avatar_url: null, status: 'active', scopes: [], connected_at: '2026-10-08T10:00:00Z' };
    apiMock.social.providers.mockResolvedValue([discord]);
    apiMock.social.accounts.mockResolvedValueOnce([]).mockResolvedValue([account]);
    apiMock.social.connectWithToken.mockResolvedValue(account);
    render(<AccountsView />);
    await userEvent.click(await screen.findByRole('button', { name: 'Connect' }));
    await userEvent.type(screen.getByLabelText('Webhook URL'), 'https://discord.com/api/webhooks/1/x');
    await userEvent.click(screen.getByRole('button', { name: 'Connect Discord' }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Connected Discord #general'));
    expect(await screen.findByRole('list', { name: 'Discord accounts' })).toHaveTextContent('Discord #general');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(apiMock.social.accounts).toHaveBeenCalledTimes(2);
  });
});

describe('AccountsView honest copy', () => {
  it('shows each unavailable network its own reason, not a blanket approval message', async () => {
    search.value = '';
    const stub = (provider: string, notes: string) =>
      normalizeProvider({ provider, configured: false, status: 'unsupported', capabilities: { requires_approval: true, notes } });
    apiMock.social.providers.mockResolvedValue([
      stub('x', 'Not available yet: X charges per post through its paid API.'),
      stub('medium', 'Not available yet: Medium reportedly no longer issues new integration tokens.'),
    ]);
    render(<AccountsView />);
    expect(await screen.findByText(/X charges per post/)).toBeInTheDocument();
    expect(screen.getByText(/Medium reportedly/)).toBeInTheDocument();
    expect(screen.queryByText(/Requires platform approval/i)).not.toBeInTheDocument();
  });

  it('offers Reconnect for an account that needs reconnecting', async () => {
    search.value = '';
    const discord = normalizeProvider({
      provider: 'discord',
      configured: true,
      status: 'supported',
      capabilities: { can_publish_text: true, connect_method: 'token', connect_fields: [{ name: 'webhook_url', label: 'Webhook URL', kind: 'url', secret: true, required: true }] },
    });
    apiMock.social.providers.mockResolvedValue([discord]);
    apiMock.social.accounts.mockResolvedValue([
      { id: 'a1', provider: 'discord', username: '#general', display_name: 'Discord #general', avatar_url: null, status: 'expired', scopes: [], connected_at: '2026-10-08T10:00:00Z' },
    ]);
    render(<AccountsView />);
    expect(await screen.findByText('Needs reconnecting')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeInTheDocument();
  });

  it('names what stays when disconnecting', async () => {
    search.value = '';
    const discord = normalizeProvider({ provider: 'discord', configured: true, status: 'supported', capabilities: { can_publish_text: true } });
    apiMock.social.providers.mockResolvedValue([discord]);
    apiMock.social.accounts.mockResolvedValue([
      { id: 'a1', provider: 'discord', username: '#general', display_name: 'Discord #general', avatar_url: null, status: 'active', scopes: [], connected_at: '2026-10-08T10:00:00Z' },
    ]);
    render(<AccountsView />);
    await userEvent.click(await screen.findByRole('button', { name: 'Disconnect Discord #general' }));
    expect(await screen.findByText('Posts scheduled for Discord #general will fail. Published posts stay on Discord.')).toBeInTheDocument();
  });
});

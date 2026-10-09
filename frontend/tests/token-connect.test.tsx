import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '@/lib/api';
import { normalizeProvider } from '@/lib/normalize';

const apiMock = vi.hoisted(() => ({ social: { connectWithToken: vi.fn() } }));
vi.mock('@/lib/api', async () => ({ ...(await vi.importActual<typeof import('@/lib/api')>('@/lib/api')), api: apiMock }));

import { TokenConnectDialog } from '@/components/token-connect-dialog';
import { useState } from 'react';

const SECRET = 'MY-SUPER-SECRET-VALUE';

const bluesky = normalizeProvider({
  provider: 'bluesky',
  configured: true,
  status: 'supported',
  capabilities: {
    can_publish_text: true,
    notes: 'Uses an app password.',
    connect_method: 'token',
    connect_fields: [
      { name: 'handle', label: 'Handle', help: 'Your handle.', placeholder: 'name.bsky.social', kind: 'text', required: true },
      { name: 'app_password', label: 'App password', help: 'Never use your main password.', kind: 'secret', secret: true, required: true },
      { name: 'pds', label: 'Server (optional)', kind: 'url', required: false },
    ],
  },
});
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

const onConnected = vi.fn();
function Harness({ provider = bluesky }: { provider?: typeof bluesky }) {
  const [open, setOpen] = useState(true);
  return (
    <>
      <button onClick={() => setOpen(true)}>reopen</button>
      <TokenConnectDialog provider={provider} open={open} onOpenChange={setOpen} onConnected={onConnected} />
    </>
  );
}

beforeEach(() => {
  apiMock.social.connectWithToken.mockReset();
  onConnected.mockReset();
});

describe('TokenConnectDialog', () => {
  it('renders the form from connect_fields with help, placeholders and a how-to link', () => {
    render(<Harness />);
    expect(screen.getByRole('dialog', { name: 'Connect Bluesky' })).toBeInTheDocument();
    expect(screen.getByLabelText('Handle')).toHaveAttribute('placeholder', 'name.bsky.social');
    expect(screen.getByLabelText('Handle')).toHaveAttribute('aria-required', 'true');
    expect(screen.getByLabelText('Server (optional)')).toHaveAttribute('type', 'url');
    expect(screen.getByText('Never use your main password.')).toBeInTheDocument();
    expect(screen.getByText('Uses an app password.')).toBeInTheDocument();
    const link = screen.getByRole('link', { name: /How to connect Bluesky/ });
    expect(link).toHaveAttribute('href', expect.stringContaining('/docs/integrations/bluesky.md'));
    expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'));
  });

  it('renders secret fields as password inputs with a show/hide toggle and no autofill', async () => {
    render(<Harness provider={discord} />);
    const input = screen.getByLabelText('Webhook URL');
    expect(input).toHaveAttribute('type', 'password');
    expect(input).toHaveAttribute('autocomplete', 'off');
    await userEvent.click(screen.getByRole('button', { name: 'Show Webhook URL' }));
    expect(input).toHaveAttribute('type', 'text');
    await userEvent.click(screen.getByRole('button', { name: 'Hide Webhook URL' }));
    expect(input).toHaveAttribute('type', 'password');
  });

  it('blocks submit with a message per required field and does not call the API', async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole('button', { name: 'Connect Bluesky' }));
    expect(await screen.findByText('Handle is required.')).toBeInTheDocument();
    expect(screen.getByText('App password is required.')).toBeInTheDocument();
    expect(screen.getByLabelText('Handle')).toHaveFocus();
    expect(apiMock.social.connectWithToken).not.toHaveBeenCalled();
  });

  it('sends trimmed values without empty optional fields, then reports the account', async () => {
    const account = { id: 'a1', provider: 'bluesky', username: 'me.bsky.social', display_name: 'Me' };
    apiMock.social.connectWithToken.mockResolvedValue(account);
    render(<Harness />);
    await userEvent.type(screen.getByLabelText('Handle'), '  me.bsky.social ');
    await userEvent.type(screen.getByLabelText('App password'), SECRET);
    await userEvent.click(screen.getByRole('button', { name: 'Connect Bluesky' }));
    await waitFor(() => expect(onConnected).toHaveBeenCalledWith(account));
    expect(apiMock.social.connectWithToken).toHaveBeenCalledWith('bluesky', { handle: 'me.bsky.social', app_password: SECRET });
  });

  it('shows server field errors next to the field and clears the secret afterwards', async () => {
    apiMock.social.connectWithToken.mockRejectedValue(new ApiError(400, 'VALIDATION_ERROR', 'Server (optional) must be an https URL', null, { pds: 'must be an https URL' }));
    render(<Harness />);
    await userEvent.type(screen.getByLabelText('Handle'), 'me.bsky.social');
    await userEvent.type(screen.getByLabelText('App password'), SECRET);
    await userEvent.type(screen.getByLabelText('Server (optional)'), 'http://x');
    await userEvent.click(screen.getByRole('button', { name: 'Connect Bluesky' }));
    const err = await screen.findByText(/Server \(optional\) must be an https address/);
    expect(screen.getByLabelText('Server (optional)')).toHaveAccessibleDescription(err.textContent ?? '');
    expect(screen.getByLabelText('Handle')).toHaveValue('me.bsky.social');
    expect(screen.getByLabelText('App password')).toHaveValue('');
    expect(onConnected).not.toHaveBeenCalled();
  });

  it.each([
    [new ApiError(400, 'VALIDATION_ERROR', 'Bluesky rejected these credentials'), /did not accept these details/],
    [new ApiError(403, 'INSUFFICIENT_SCOPE', 'missing scope'), /social:connect/],
    [new ApiError(429, 'RATE_LIMITED', 'slow down'), /Too many attempts/],
    [new ApiError(502, 'PROVIDER_ERROR', 'boom'), /could not be reached/],
    [new ApiError(500, 'INTERNAL', 'INTERNAL'), /Steerpost had a problem/],
  ])('maps %# to a plain sentence without leaking input', async (error, expected) => {
    apiMock.social.connectWithToken.mockRejectedValue(error);
    render(<Harness />);
    await userEvent.type(screen.getByLabelText('Handle'), 'me.bsky.social');
    await userEvent.type(screen.getByLabelText('App password'), SECRET);
    await userEvent.click(screen.getByRole('button', { name: 'Connect Bluesky' }));
    const alert = await screen.findByText(expected);
    expect(alert.closest('[role=alert]')).not.toBeNull();
    expect(document.body.textContent).not.toContain(SECRET);
  });

  it('leaves no secret in the DOM or on reopen after the dialog is closed', async () => {
    render(<Harness provider={discord} />);
    const secret = 'https://discord.com/api/webhooks/1/' + SECRET;
    await userEvent.type(screen.getByLabelText('Webhook URL'), secret);
    await userEvent.click(screen.getByRole('button', { name: 'Show Webhook URL' }));
    expect(screen.getByLabelText('Webhook URL')).toHaveValue(secret);
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(document.body.innerHTML).not.toContain(SECRET);
    await userEvent.click(screen.getByRole('button', { name: 'reopen' }));
    const again = within(screen.getByRole('dialog')).getByLabelText('Webhook URL');
    expect(again).toHaveValue('');
    expect(again).toHaveAttribute('type', 'password');
    expect(window.localStorage.length).toBe(0);
    expect(window.location.href).not.toContain(SECRET);
  });
});

describe('TokenConnectDialog storage note', () => {
  it('does not promise HTTPS on a plain http page (local installs)', () => {
    render(<TokenConnectDialog provider={discord} open onOpenChange={() => {}} onConnected={() => {}} />);
    const text = screen.getByRole('dialog').textContent ?? '';
    expect(text).toContain('Steerpost stores these details encrypted and never shows them again.');
    expect(text).not.toContain('HTTPS');
  });

  it('mentions HTTPS only when the page is served over HTTPS', () => {
    const original = window.location;
    Object.defineProperty(window, 'location', { value: { ...original, protocol: 'https:' }, configurable: true });
    try {
      render(<TokenConnectDialog provider={discord} open onOpenChange={() => {}} onConnected={() => {}} />);
      expect(screen.getByRole('dialog').textContent).toContain('over HTTPS');
    } finally {
      Object.defineProperty(window, 'location', { value: original, configurable: true });
    }
  });
});

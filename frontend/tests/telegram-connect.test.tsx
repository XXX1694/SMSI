import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ToastProvider } from '@/components/toast';
import { TelegramConnect } from '@/components/telegram-connect';

const apiMock = vi.hoisted(() => ({
  social: { startTelegramLink: vi.fn(), telegramLinkStatus: vi.fn() },
}));
vi.mock('@/lib/api', () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {
    status: number;
    code: string;
    requestId: string | null = null;
    constructor(status: number, code: string, message: string) {
      super(message);
      this.status = status;
      this.code = code;
    }
  },
}));

import { ApiError } from '@/lib/api';

const NOW = Date.parse('2026-10-07T12:00:00.000Z');
const link = (n = 1, ttlSeconds = 900) => ({
  id: `link-${n}`,
  code: `SOS-CODE000${n}`.replace('0', 'A'),
  expires_at: new Date(NOW + ttlSeconds * 1000).toISOString(),
  bot_username: 'socialos_bot',
  instructions: 'x',
});
const account = { id: 'acc-1', provider: 'telegram', username: 'e2e_channel', display_name: 'E2E Channel', avatar_url: null, status: 'active', scopes: [], connected_at: '2026-10-07T12:00:05Z' };

// userEvent's async wrapper waits on a real setTimeout(0) and hangs under vitest fake timers, so use fireEvent.
const click = async (name: string) => {
  fireEvent.click(screen.getByRole('button', { name }));
  await advance(0);
};
const advance = (ms: number) => act(async () => { await vi.advanceTimersByTimeAsync(ms); });

function setup(onConnected = vi.fn()) {
  render(
    <ToastProvider>
      <TelegramConnect onConnected={onConnected} />
    </ToastProvider>,
  );
  return onConnected;
}

const start = () => click('Connect channel');

describe('TelegramConnect', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date'] });
    vi.setSystemTime(NOW);
    apiMock.social.startTelegramLink.mockReset();
    apiMock.social.telegramLinkStatus.mockReset();
    apiMock.social.startTelegramLink.mockResolvedValue(link());
    apiMock.social.telegramLinkStatus.mockResolvedValue({ status: 'pending', account: null });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('has no channel field: connecting starts with a code, not a name', () => {
    setup();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect channel' })).toBeEnabled();
  });

  it('shows the three numbered steps with the bot, a copyable code and a countdown', async () => {
    setup();
    await start();

    expect(apiMock.social.startTelegramLink).toHaveBeenCalledTimes(1);
    const steps = screen.getAllByRole('listitem');
    expect(steps).toHaveLength(3);
    expect(steps[0]).toHaveTextContent('Add @socialos_bot as an administrator');
    expect(steps[0]).toHaveTextContent('Post messages');
    expect(steps[1]).toHaveTextContent('Post this code there');
    expect(screen.getByLabelText('One-time code')).toHaveTextContent('SOS-CODEA001');
    expect(screen.getByRole('timer')).toHaveTextContent('Expires in 15:00');
    expect(steps[2]).toHaveTextContent('Waiting for the code to appear');
    expect(steps[2]).toHaveTextContent('every 2 seconds');
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
  });

  it('copies the code to the clipboard', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    setup();
    await start();
    await click('Copy code');
    expect(writeText).toHaveBeenCalledWith('SOS-CODEA001');
  });

  it('counts down every second and warns in the last minute', async () => {
    setup();
    await start();
    await advance(61_000);
    expect(screen.getByRole('timer')).toHaveTextContent('Expires in 13:59');
    expect(screen.getByRole('timer')).not.toHaveClass('text-warning');
    await advance(780_000);
    expect(screen.getByRole('timer')).toHaveTextContent('Expires in 0:59');
    expect(screen.getByRole('timer')).toHaveClass('text-warning');
  });

  it('polls the status every 2 seconds', async () => {
    setup();
    await start();
    expect(apiMock.social.telegramLinkStatus).not.toHaveBeenCalled();
    await advance(1999);
    expect(apiMock.social.telegramLinkStatus).not.toHaveBeenCalled();
    await advance(1);
    expect(apiMock.social.telegramLinkStatus).toHaveBeenCalledTimes(1);
    expect(apiMock.social.telegramLinkStatus).toHaveBeenLastCalledWith('link-1');
    await advance(2000);
    expect(apiMock.social.telegramLinkStatus).toHaveBeenCalledTimes(2);
    await advance(6000);
    expect(apiMock.social.telegramLinkStatus).toHaveBeenCalledTimes(5);
  });

  it('shows the connected channel, refreshes the accounts list once and stops polling', async () => {
    apiMock.social.telegramLinkStatus
      .mockResolvedValueOnce({ status: 'pending', account: null })
      .mockResolvedValue({ status: 'connected', account });
    const onConnected = setup();
    await start();
    await advance(2000);
    expect(onConnected).not.toHaveBeenCalled();
    expect(screen.getByLabelText('One-time code')).toBeInTheDocument();

    await advance(2000);
    expect(onConnected).toHaveBeenCalledTimes(1);
    expect(screen.getByText('E2E Channel')).toBeInTheDocument();
    expect(screen.getByText(/You can now publish to it/)).toBeInTheDocument();
    expect(screen.getByText('@e2e_channel', { exact: false })).toBeInTheDocument();
    expect(screen.queryByLabelText('One-time code')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Connect another' })).toBeInTheDocument();

    await advance(20_000);
    expect(apiMock.social.telegramLinkStatus).toHaveBeenCalledTimes(2);
    expect(onConnected).toHaveBeenCalledTimes(1);
  });

  it('offers a new code when the server reports the code as expired', async () => {
    apiMock.social.telegramLinkStatus.mockResolvedValueOnce({ status: 'expired', account: null });
    apiMock.social.startTelegramLink.mockResolvedValueOnce(link(1)).mockResolvedValueOnce(link(2));
    setup();
    await start();
    await advance(2000);
    expect(screen.getByText(/This code has expired/)).toBeInTheDocument();
    expect(screen.queryByLabelText('One-time code')).not.toBeInTheDocument();
    await advance(10_000);
    expect(apiMock.social.telegramLinkStatus).toHaveBeenCalledTimes(1); // no polling once expired

    await click('Get a new code');
    expect(apiMock.social.startTelegramLink).toHaveBeenCalledTimes(2);
    expect(screen.getByLabelText('One-time code')).toHaveTextContent('SOS-CODEA002');
    await advance(2000);
    expect(apiMock.social.telegramLinkStatus).toHaveBeenLastCalledWith('link-2');
  });

  it('expires on its own when the countdown ends', async () => {
    apiMock.social.startTelegramLink.mockResolvedValue(link(1, 6));
    setup();
    await start();
    await advance(5000);
    expect(screen.getByRole('timer')).toHaveTextContent('Expires in 0:01');
    await advance(1000);
    expect(screen.getByText(/This code has expired/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Get a new code' })).toBeInTheDocument();
  });

  it('treats a 404 for the link as expired', async () => {
    apiMock.social.telegramLinkStatus.mockRejectedValueOnce(new ApiError(404, 'NOT_FOUND', 'not found'));
    setup();
    await start();
    await advance(2000);
    expect(screen.getByText(/This code has expired/)).toBeInTheDocument();
  });

  it('keeps polling through network errors and says so', async () => {
    apiMock.social.telegramLinkStatus
      .mockRejectedValueOnce(new ApiError(0, 'NETWORK', 'offline'))
      .mockRejectedValueOnce(new ApiError(502, 'INTERNAL', 'bad gateway'))
      .mockResolvedValue({ status: 'connected', account });
    const onConnected = setup();
    await start();
    await advance(2000);
    expect(screen.getByText(/Having trouble reaching the server/)).toBeInTheDocument();
    await advance(2000);
    expect(screen.getByText(/Having trouble reaching the server/)).toBeInTheDocument();
    await advance(2000);
    expect(onConnected).toHaveBeenCalledTimes(1);
    expect(screen.getByText('E2E Channel')).toBeInTheDocument();
  });

  it('stops and explains when the session is gone', async () => {
    apiMock.social.telegramLinkStatus.mockRejectedValue(new ApiError(401, 'UNAUTHENTICATED', 'Please sign in.'));
    setup();
    await start();
    await advance(2000);
    expect(screen.getByRole('alert')).toHaveTextContent('Please sign in.');
    await advance(10_000);
    expect(apiMock.social.telegramLinkStatus).toHaveBeenCalledTimes(1);
  });

  it('reports a failure to create the code and allows a retry', async () => {
    apiMock.social.startTelegramLink
      .mockRejectedValueOnce(new ApiError(501, 'PROVIDER_NOT_AVAILABLE', 'Telegram is not configured on this server'))
      .mockResolvedValueOnce(link());
    setup();
    await start();
    expect(screen.getByRole('alert')).toHaveTextContent('Telegram is not configured on this server');
    expect(screen.queryByLabelText('One-time code')).not.toBeInTheDocument();
    await start();
    expect(screen.getByLabelText('One-time code')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('cancel stops polling and returns to the start', async () => {
    setup();
    await start();
    await advance(2000);
    await click('Cancel');
    expect(screen.getByRole('button', { name: 'Connect channel' })).toBeInTheDocument();
    await advance(10_000);
    expect(apiMock.social.telegramLinkStatus).toHaveBeenCalledTimes(1);
  });
});

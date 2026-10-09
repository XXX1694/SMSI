'use client';
import { CheckCircle2, Loader2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { CopyButton } from '@/components/developer/copy-block';
import { Notice } from '@/components/states';
import { useToast } from '@/components/toast';
import { Button } from '@/components/ui/button';
import { ApiError, api } from '@/lib/api';
import { DEMO } from '@/lib/demo/config';
import {
  LINK_POLL_INTERVAL_MS,
  formatCountdown,
  isExpiringSoon,
  linkPhase,
  secondsLeft,
  shouldPoll,
} from '@/lib/telegram-link';
import type { SocialAccount, TelegramLink, TelegramLinkStatus } from '@/lib/types';
import { errorMessage } from '@/hooks';
import { cn } from '@/lib/utils';

/**
 * Connects a Telegram channel or group. There is no field for a channel name: the
 * user adds the Steerpost bot as admin and posts a one-time code in the chat, which
 * proves they control it. The screen then polls until the bot has seen the code.
 */
export function TelegramConnect({ onConnected }: { onConnected: () => void }) {
  const toast = useToast();
  const [link, setLink] = useState<TelegramLink | null>(null);
  const [done, setDone] = useState<{ name: string; handle: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function start() {
    setBusy(true);
    setError(null);
    try {
      setLink(await api.social.startTelegramLink());
      setDone(null);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  function connected(account: SocialAccount | null) {
    const name = account?.display_name || account?.username || 'your chat';
    setLink(null);
    setDone({ name, handle: account?.display_name ? (account.username ?? '') : '' });
    toast.success(`Connected ${name}`);
    onConnected();
  }

  if (link) {
    return (
      <LinkSteps
        key={link.id}
        link={link}
        busy={busy}
        error={error}
        onConnected={connected}
        onCancel={() => setLink(null)}
        onNewCode={() => void start()}
      />
    );
  }

  if (done) {
    return (
      <div
        role="status"
        className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-success/30 bg-success-soft px-3 py-2.5 text-sm"
      >
        <span className="flex min-w-0 items-center gap-2">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-success" aria-hidden />
          <span className="min-w-0">
            Connected <strong className="font-medium">{done.name}</strong>
            {done.handle ? <span className="text-muted-foreground"> · @{done.handle.replace(/^@/, '')}</span> : null}. You can now publish to it.
          </span>
        </span>
        <Button variant="secondary" size="sm" onClick={() => void start()} disabled={busy}>
          {busy ? 'Creating code…' : 'Connect another'}
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-3 rounded-md border bg-surface p-4">
      <div>
        <p className="text-sm font-medium">Connect a channel or group</p>
        <p className="mt-0.5 text-xs text-muted-foreground">
          You prove you control it by posting a one-time code there, so only chats you own can be connected.
        </p>
      </div>
      {error ? <Notice tone="danger">{error}</Notice> : null}
      <Button onClick={() => void start()} disabled={busy}>
        {busy ? 'Creating code…' : 'Connect channel'}
      </Button>
    </div>
  );
}

function StepNumber({ n }: { n: number }) {
  return (
    <span
      aria-hidden
      className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-accent-soft text-xs font-semibold text-accent"
    >
      {n}
    </span>
  );
}

interface LinkStepsProps {
  link: TelegramLink;
  busy: boolean;
  error: string | null;
  onConnected: (account: SocialAccount | null) => void;
  onCancel: () => void;
  onNewCode: () => void;
}

function LinkSteps({ link, busy, error, onConnected, onCancel, onNewCode }: LinkStepsProps) {
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [server, setServer] = useState<TelegramLinkStatus | null>(null);
  const [account, setAccount] = useState<SocialAccount | null>(null);
  const [retrying, setRetrying] = useState(false);
  const [fatal, setFatal] = useState<string | null>(null);
  const remaining = secondsLeft(link.expires_at, nowMs);
  const phase = linkPhase(server, remaining);
  const bot = link.bot_username ? `@${link.bot_username}` : 'the Steerpost bot';

  // Countdown: re-render once a second while waiting.
  useEffect(() => {
    if (phase !== 'waiting') return undefined;
    setNowMs(Date.now());
    const t = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, [phase]);

  // Status: poll every 2 s until the bot has seen the code (or it expired).
  useEffect(() => {
    if (!shouldPoll(phase) || fatal) return undefined;
    let cancelled = false;
    let timer: number | undefined;
    const poll = async () => {
      try {
        const st = await api.social.telegramLinkStatus(link.id);
        if (cancelled) return;
        setRetrying(false);
        if (st.status !== 'pending') {
          setAccount(st.account);
          setServer(st.status);
          return;
        }
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError && e.status === 404) {
          setServer('expired'); // the server no longer knows this code
          return;
        }
        if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
          setFatal(errorMessage(e));
          return;
        }
        setRetrying(true); // network blip or 5xx: keep trying
      }
      timer = window.setTimeout(() => void poll(), LINK_POLL_INTERVAL_MS);
    };
    timer = window.setTimeout(() => void poll(), LINK_POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [link.id, phase, fatal]);

  // Hand the result to the parent exactly once.
  const reported = useRef(false);
  useEffect(() => {
    if (phase === 'connected' && !reported.current) {
      reported.current = true;
      onConnected(account);
    }
  }, [phase, account, onConnected]);

  if (phase === 'connected') return null;

  if (fatal) {
    return (
      <div className="space-y-3">
        <Notice tone="danger">{fatal}</Notice>
        <Button variant="secondary" size="sm" onClick={onCancel}>
          Close
        </Button>
      </div>
    );
  }

  if (phase === 'expired') {
    return (
      <div className="space-y-3">
        <Notice tone="warning">This code has expired. Codes work for 15 minutes and only once.</Notice>
        {error ? <Notice tone="danger">{error}</Notice> : null}
        <div className="flex flex-wrap gap-2">
          <Button onClick={onNewCode} disabled={busy}>
            {busy ? 'Creating code…' : 'Get a new code'}
          </Button>
          <Button variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4 rounded-md border bg-surface p-4">
      <ol className="space-y-4" aria-label="Connect a Telegram channel or group">
        <li className="flex gap-3">
          <StepNumber n={1} />
          <div className="min-w-0 space-y-1.5">
            <p className="text-sm">
              Add <strong className="font-medium">{bot}</strong> as an administrator of your channel or group with the{' '}
              <strong className="font-medium">&ldquo;Post messages&rdquo;</strong> right.
            </p>
            {link.bot_username ? <CopyButton text={`@${link.bot_username}`} label="Copy bot name" /> : null}
          </div>
        </li>
        <li className="flex gap-3">
          <StepNumber n={2} />
          <div className="min-w-0 space-y-2">
            <p className="text-sm">Post this code there as a normal message:</p>
            <div className="flex flex-wrap items-center gap-2">
              <code
                aria-label="Link code"
                className="select-all rounded-md border bg-muted px-3 py-1.5 font-mono text-base font-semibold tracking-wider"
              >
                {link.code}
              </code>
              <CopyButton text={link.code} label="Copy code" />
            </div>
            <p
              role="timer"
              className={cn('text-xs', isExpiringSoon(remaining) ? 'font-medium text-warning' : 'text-muted-foreground')}
            >
              Expires in <span className="tabular-nums">{formatCountdown(remaining)}</span>
            </p>
          </div>
        </li>
        <li className="flex gap-3">
          <StepNumber n={3} />
          <div className="min-w-0 space-y-1" aria-live="polite">
            <p className="flex items-center gap-2 text-sm">
              <Loader2 className="h-4 w-4 shrink-0 animate-spin text-accent motion-reduce:animate-none" aria-hidden />
              Waiting for the code to appear in your chat…
            </p>
            <p className="text-xs text-muted-foreground">
              {retrying
                ? 'Having trouble reaching the server. Retrying…'
                : 'We check every 2 seconds. Keep this page open; the bot deletes the code message once you are connected.'}
            </p>
            {DEMO ? (
              <p className="text-xs text-muted-foreground">
                Demo: no real Telegram chat is needed. The code is recognised automatically after a few seconds.
              </p>
            ) : null}
          </div>
        </li>
      </ol>
      <Button variant="ghost" size="sm" onClick={onCancel}>
        Cancel
      </Button>
    </div>
  );
}

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
import { useErrorText } from '@/hooks';
import { nodes } from '@/i18n/rich';
import { cn } from '@/lib/utils';
import { useTranslations } from '@/i18n/use-translations';

/**
 * Connects a Telegram channel or group. There is no field for a channel name: the
 * user adds the Steerpost bot as admin and posts a one-time code in the chat, which
 * proves they control it. The screen then polls until the bot has seen the code.
 */
export function TelegramConnect({ onConnected }: { onConnected: () => void }) {
  const t = useTranslations('accounts.telegram');
  const ta = useTranslations('accounts');
  const errorText = useErrorText();
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
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  function connected(account: SocialAccount | null) {
    const name = account?.display_name || account?.username || t('yourChat');
    setLink(null);
    setDone({ name, handle: account?.display_name ? (account.username ?? '') : '' });
    toast.success(ta('connected', { name }));
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
            {nodes(
              t.rich('connectedTo', {
                name: done.name,
                hasHandle: String(Boolean(done.handle)),
                handle: done.handle.replace(/^@/, ''),
                b: (c) => <strong className="font-medium">{c}</strong>,
                muted: (c) => <span className="text-muted-foreground">{c}</span>,
              }),
            )}
          </span>
        </span>
        <Button variant="secondary" size="sm" onClick={() => void start()} disabled={busy}>
          {busy ? t('creatingCode') : ta('connectAnother')}
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-3 rounded-md border bg-surface p-4">
      <div>
        <p className="text-sm font-medium">{t('connectTitle')}</p>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {t('connectBody')}
        </p>
      </div>
      {error ? <Notice tone="danger">{error}</Notice> : null}
      <Button onClick={() => void start()} disabled={busy}>
        {busy ? t('creatingCode') : t('connectChannel')}
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
  const t = useTranslations('accounts.telegram');
  const tc = useTranslations('common');
  const errorText = useErrorText();
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [server, setServer] = useState<TelegramLinkStatus | null>(null);
  const [account, setAccount] = useState<SocialAccount | null>(null);
  const [retrying, setRetrying] = useState(false);
  const [fatal, setFatal] = useState<string | null>(null);
  const remaining = secondsLeft(link.expires_at, nowMs);
  const phase = linkPhase(server, remaining);
  const bot = link.bot_username ? `@${link.bot_username}` : t('botFallback');

  // Countdown: re-render once a second while waiting.
  useEffect(() => {
    if (phase !== 'waiting') return undefined;
    setNowMs(Date.now());
    const timer = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(timer);
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
          setFatal(errorText(e));
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
  }, [link.id, phase, fatal, errorText]);

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
          {tc('close')}
        </Button>
      </div>
    );
  }

  if (phase === 'expired') {
    return (
      <div className="space-y-3">
        <Notice tone="warning">{t('expired')}</Notice>
        {error ? <Notice tone="danger">{error}</Notice> : null}
        <div className="flex flex-wrap gap-2">
          <Button onClick={onNewCode} disabled={busy}>
            {busy ? t('creatingCode') : t('newCode')}
          </Button>
          <Button variant="ghost" onClick={onCancel}>
            {tc('cancel')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4 rounded-md border bg-surface p-4">
      <ol className="space-y-4" aria-label={t('stepsLabel')}>
        <li className="flex gap-3">
          <StepNumber n={1} />
          <div className="min-w-0 space-y-1.5">
            <p className="text-sm">
              {nodes(t.rich('step1', { bot, b: (c) => <strong className="font-medium">{c}</strong> }))}
            </p>
            {link.bot_username ? <CopyButton text={`@${link.bot_username}`} label={t('copyBot')} /> : null}
          </div>
        </li>
        <li className="flex gap-3">
          <StepNumber n={2} />
          <div className="min-w-0 space-y-2">
            <p className="text-sm">{t('step2')}</p>
            <div className="flex flex-wrap items-center gap-2">
              <code
                aria-label={t('codeLabel')}
                className="select-all rounded-md border bg-muted px-3 py-1.5 font-mono text-base font-semibold tracking-wider"
              >
                {link.code}
              </code>
              <CopyButton text={link.code} label={t('copyCode')} />
            </div>
            <p
              role="timer"
              className={cn('text-xs', isExpiringSoon(remaining) ? 'font-medium text-warning' : 'text-muted-foreground')}
            >
              {nodes(t.rich('expiresIn', { remaining: formatCountdown(remaining), time: (c) => <span className="tabular-nums">{c}</span> }))}
            </p>
          </div>
        </li>
        <li className="flex gap-3">
          <StepNumber n={3} />
          <div className="min-w-0 space-y-1" aria-live="polite">
            <p className="flex items-center gap-2 text-sm">
              <Loader2 className="h-4 w-4 shrink-0 animate-spin text-accent motion-reduce:animate-none" aria-hidden />
              {t('waiting')}
            </p>
            <p className="text-xs text-muted-foreground">
              {retrying ? t('retrying') : t('pollNote')}
            </p>
            {DEMO ? (
              <p className="text-xs text-muted-foreground">
                {t('demoNote')}
              </p>
            ) : null}
          </div>
        </li>
      </ol>
      <Button variant="ghost" size="sm" onClick={onCancel}>
        {tc('cancel')}
      </Button>
    </div>
  );
}

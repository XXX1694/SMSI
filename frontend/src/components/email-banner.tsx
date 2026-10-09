'use client';
import { MailWarning, X } from 'lucide-react';
import { useState } from 'react';
import { useAuth } from '@/components/auth-provider';
import { Button } from '@/components/ui/button';
import { useErrorText } from '@/hooks';
import { nodes } from '@/i18n/rich';
import { useTranslations } from '@/i18n/use-translations';
import { api } from '@/lib/api';
import { readStorage, writeStorage } from '@/lib/storage';

const DISMISS_KEY = 'socialos_mail_notice_dismissed';

type Resend = { state: 'idle' | 'sending' | 'sent' } | { state: 'error'; message: string };

/**
 * Two honest notices above every signed-in screen:
 *  - the server enforces email verification and this address is not verified yet (with a resend button);
 *  - the server does not deliver email at all, so verification and password-reset mail never arrives.
 */
export function EmailBanner() {
  const { user } = useAuth();
  const t = useTranslations('shell.emailBanner');
  const tc = useTranslations('common');
  const errorText = useErrorText();
  const [resend, setResend] = useState<Resend>({ state: 'idle' });
  const [dismissed, setDismissed] = useState(() => readStorage(DISMISS_KEY) === '1');
  if (!user) return null;

  if (!user.email_verified && user.verification_enforced) {
    const send = async () => {
      setResend({ state: 'sending' });
      try {
        await api.auth.resendVerification();
        setResend({ state: 'sent' });
      } catch (e) {
        setResend({ state: 'error', message: errorText(e) });
      }
    };
    return (
      <div role="alert" className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-warning/30 bg-warning-soft px-4 py-2.5 text-sm md:px-10">
        <MailWarning className="hidden h-4 w-4 shrink-0 sm:block" aria-hidden />
        <p className="min-w-0 flex-1 basis-64">
          <span className="font-medium">{t('verifyHeading')}</span>{' '}
          {nodes(t.rich('verifyBody', { email: user.email, addr: (c) => <span className="break-words">{c}</span> }))}
        </p>
        {resend.state === 'sent' ? (
          <span role="status" className="text-muted-foreground">
            {t('sent')}
          </span>
        ) : (
          <Button variant="secondary" size="sm" onClick={() => void send()} disabled={resend.state === 'sending'}>
            {resend.state === 'sending' ? tc('sending') : t('resend')}
          </Button>
        )}
        {resend.state === 'error' ? <span className="w-full text-danger">{resend.message}</span> : null}
      </div>
    );
  }

  if (user.mail_delivery === 'log' && !dismissed) {
    return (
      <div role="note" className="flex items-center gap-3 border-b bg-muted px-4 py-2.5 text-sm md:px-10">
        <p className="min-w-0 flex-1">
          <span className="font-medium">{t('logHeading')}</span> {t('logBody')}
        </p>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0"
          aria-label={t('dismiss')}
          onClick={() => {
            writeStorage(DISMISS_KEY, '1');
            setDismissed(true);
          }}
        >
          <X className="h-4 w-4" aria-hidden />
        </Button>
      </div>
    );
  }
  return null;
}

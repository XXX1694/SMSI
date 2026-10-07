'use client';
import { useState, type FormEvent } from 'react';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { useToast } from '@/components/toast';
import { api } from '@/lib/api';
import { errorMessage } from '@/hooks';

export function TelegramConnect({ onConnected }: { onConnected: () => void }) {
  const [chat, setChat] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  async function submit(e: FormEvent) {
    e.preventDefault();
    const value = chat.trim();
    if (!/^(@[A-Za-z][A-Za-z0-9_]{3,}|-?\d{5,})$/.test(value)) {
      setError('Enter a channel like @mychannel or a numeric chat id.');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.social.connectTelegram(value);
      toast.success(`Connected ${value}`);
      setChat('');
      onConnected();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-3 sm:flex-row sm:items-end" noValidate>
      <div className="flex-1">
        <Field
          label="Telegram channel"
          htmlFor="tg-chat"
          hint="Add the SocialOS bot as a channel admin first, then enter @username or chat id."
          error={error}
        >
          <Input id="tg-chat" placeholder="@mychannel" value={chat} onChange={(e) => setChat(e.target.value)} aria-invalid={error ? true : undefined} />
        </Field>
      </div>
      <Button type="submit" disabled={busy || !chat.trim()}>
        {busy ? 'Verifying…' : 'Connect channel'}
      </Button>
    </form>
  );
}

'use client';
import Link from 'next/link';
import { useRouter, useSearchParams } from 'next/navigation';
import { useState, type FormEvent } from 'react';
import { useAuth } from '@/components/auth-provider';
import { Button } from '@/components/ui/button';
import { Field, Input } from '@/components/ui/input';
import { DEMO, DEMO_EMAIL, DEMO_PASSWORD } from '@/lib/demo/config';
import { errorMessage } from '@/hooks';

export function AuthForm({ mode }: { mode: 'login' | 'register' }) {
  const { login, register } = useAuth();
  const router = useRouter();
  const params = useSearchParams();
  // The demo has a single built-in user, so its credentials are pre-filled.
  const [email, setEmail] = useState(DEMO && mode === 'login' ? DEMO_EMAIL : '');
  const [password, setPassword] = useState(DEMO && mode === 'login' ? DEMO_PASSWORD : '');
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const isLogin = mode === 'login';

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (!isLogin && password.length < 8) {
      setError('Password must be at least 8 characters.');
      return;
    }
    setBusy(true);
    try {
      if (isLogin) await login(email.trim(), password);
      else await register(email.trim(), password, name.trim());
      const next = params.get('next');
      router.replace(next && next.startsWith('/') && !next.startsWith('//') ? next : '/dashboard');
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <h1 className="text-xl font-semibold tracking-tight">{isLogin ? 'Sign in to SocialOS' : 'Create your account'}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {isLogin ? 'Welcome back.' : 'Start composing and scheduling in a minute.'}
        </p>
        {DEMO ? (
          <p className="mt-3 rounded-md border bg-muted px-3 py-2 text-xs text-muted-foreground">
            This is a demo with a built-in account, so the form is already filled in. Just press Sign in.
          </p>
        ) : null}
        <form onSubmit={submit} className="mt-8 space-y-4" noValidate>
          {!isLogin ? (
            <Field label="Name" htmlFor="name">
              <Input id="name" autoComplete="name" required value={name} onChange={(e) => setName(e.target.value)} />
            </Field>
          ) : null}
          <Field label="Email" htmlFor="email">
            <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </Field>
          <Field label="Password" htmlFor="password" hint={isLogin ? undefined : 'At least 8 characters.'}>
            <Input
              id="password"
              type="password"
              autoComplete={isLogin ? 'current-password' : 'new-password'}
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          {error ? (
            <p role="alert" className="text-sm text-danger">
              {error}
            </p>
          ) : null}
          <Button type="submit" className="w-full" disabled={busy || !email || !password}>
            {busy ? 'Please wait…' : isLogin ? 'Sign in' : 'Create account'}
          </Button>
        </form>
        <p className="mt-6 text-sm text-muted-foreground">
          {isLogin ? (
            <>
              No account?{' '}
              <Link href="/register" className="text-accent hover:underline">
                Register
              </Link>
            </>
          ) : (
            <>
              Already registered?{' '}
              <Link href="/login" className="text-accent hover:underline">
                Sign in
              </Link>
            </>
          )}
        </p>
      </div>
    </main>
  );
}

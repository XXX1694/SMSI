'use client';
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, ApiError, setCsrfToken } from '@/lib/api';
import type { Me } from '@/lib/types';

interface AuthState {
  user: Me | null;
  loading: boolean;
  /** /me failed for a reason other than "not signed in" (server down, offline). The user is NOT signed out. */
  error: unknown;
  /** Re-runs the /me check after such a failure. */
  retry: () => void;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, displayName: string, acceptTerms: boolean) => Promise<void>;
  /** Finishes a provider sign-up after the Terms were accepted and signs the new user in. */
  completeSignup: (displayName: string, acceptTerms: boolean) => Promise<void>;
  logout: () => Promise<void>;
  /** Re-reads /me, e.g. after the email was verified. A failure keeps the current user. */
  refresh: () => Promise<void>;
  /** Forgets the user without calling the server, after it ended the session itself (account deletion). */
  endSession: (reason?: 'deleted') => void;
  /** Why the session ended without the user signing out, so the login page can say so. */
  endedBy: 'deleted' | null;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const [attempt, setAttempt] = useState(0);
  const [endedBy, setEndedBy] = useState<'deleted' | null>(null);

  const adopt = useCallback((me: Me | null) => {
    setCsrfToken(me?.csrf_token ?? null);
    setUser(me);
  }, []);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    api.auth.me().then(
      (me) => {
        if (!cancelled) {
          adopt(me);
          setLoading(false);
        }
      },
      (e: unknown) => {
        if (cancelled) return;
        // Only a 401 means "signed out". A 500 or a dropped connection must not look like one.
        if (e instanceof ApiError && e.status === 401) adopt(null);
        else setError(e);
        setLoading(false);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [adopt, attempt]);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);

  const login = useCallback(
    async (email: string, password: string) => {
      adopt(await api.auth.login({ email, password }));
    },
    [adopt],
  );
  const register = useCallback(
    async (email: string, password: string, displayName: string, acceptTerms: boolean) => {
      adopt(await api.auth.register({ email, password, display_name: displayName, accept_terms: acceptTerms }));
    },
    [adopt],
  );
  const completeSignup = useCallback(
    async (displayName: string, acceptTerms: boolean) => {
      adopt(await api.auth.completeSignup({ display_name: displayName, accept_terms: acceptTerms }));
    },
    [adopt],
  );
  const logout = useCallback(async () => {
    try {
      await api.auth.logout();
    } finally {
      adopt(null);
    }
  }, [adopt]);

  const refresh = useCallback(async () => {
    try {
      adopt(await api.auth.me());
    } catch {
      /* keep the current user; the next request surfaces a real sign-out */
    }
  }, [adopt]);

  const endSession = useCallback(
    (reason?: 'deleted') => {
      setEndedBy(reason ?? null);
      adopt(null);
    },
    [adopt],
  );

  const value = useMemo(
    () => ({ user, loading, error, retry, login, register, completeSignup, logout, refresh, endSession, endedBy }),
    [user, loading, error, retry, login, register, completeSignup, logout, refresh, endSession, endedBy],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

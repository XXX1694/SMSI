'use client';
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, setCsrfToken } from '@/lib/api';
import type { Me } from '@/lib/types';

interface AuthState {
  user: Me | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, displayName: string) => Promise<void>;
  logout: () => Promise<void>;
  /** Re-reads /me, e.g. after the email was verified. A failure keeps the current user. */
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  const adopt = useCallback((me: Me | null) => {
    setCsrfToken(me?.csrf_token ?? null);
    setUser(me);
  }, []);

  useEffect(() => {
    let cancelled = false;
    api.auth.me().then(
      (me) => {
        if (!cancelled) {
          adopt(me);
          setLoading(false);
        }
      },
      () => {
        if (!cancelled) {
          adopt(null);
          setLoading(false);
        }
      },
    );
    return () => {
      cancelled = true;
    };
  }, [adopt]);

  const login = useCallback(
    async (email: string, password: string) => {
      adopt(await api.auth.login({ email, password }));
    },
    [adopt],
  );
  const register = useCallback(
    async (email: string, password: string, displayName: string) => {
      adopt(await api.auth.register({ email, password, display_name: displayName }));
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

  const value = useMemo(
    () => ({ user, loading, login, register, logout, refresh }),
    [user, loading, login, register, logout, refresh],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

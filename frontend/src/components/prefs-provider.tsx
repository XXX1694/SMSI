'use client';
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { readStorage, writeStorage } from '@/lib/storage';
import { browserTimezone, isValidTimezone } from '@/lib/time';
import { MOTION_OFF_CLASS } from '@/lib/view-transition';

export type Theme = 'light' | 'dark' | 'system';

interface Prefs {
  timezone: string;
  setTimezone: (tz: string) => void;
  theme: Theme;
  setTheme: (t: Theme) => void;
  /** Pause motion: animations off in this browser, on top of the system's reduced-motion setting. */
  motionPaused: boolean;
  setMotionPaused: (paused: boolean) => void;
}

/** Read before first paint by the inline script in app/layout.tsx, so nothing animates before React hydrates. */
export const MOTION_KEY = 'socialos_motion';

const PrefsContext = createContext<Prefs | null>(null);

function applyTheme(theme: Theme): void {
  const dark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.classList.toggle('dark', dark);
}

export function PrefsProvider({ children }: { children: ReactNode }) {
  const [timezone, setTz] = useState('UTC');
  const [theme, setThemeState] = useState<Theme>('system');
  const [motionPaused, setMotionPausedState] = useState(false);

  useEffect(() => {
    const tz = readStorage('socialos_tz');
    setTz(tz && isValidTimezone(tz) ? tz : browserTimezone());
    const t = readStorage('socialos_theme');
    if (t === 'light' || t === 'dark' || t === 'system') setThemeState(t);
    setMotionPausedState(readStorage(MOTION_KEY) === 'off');
  }, []);

  const setTimezone = useCallback((tz: string) => {
    if (!isValidTimezone(tz)) return;
    setTz(tz);
    writeStorage('socialos_tz', tz);
  }, []);

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t);
    writeStorage('socialos_theme', t);
    applyTheme(t);
  }, []);

  const setMotionPaused = useCallback((paused: boolean) => {
    setMotionPausedState(paused);
    writeStorage(MOTION_KEY, paused ? 'off' : 'on');
    document.documentElement.classList.toggle(MOTION_OFF_CLASS, paused);
  }, []);

  const value = useMemo(
    () => ({ timezone, setTimezone, theme, setTheme, motionPaused, setMotionPaused }),
    [timezone, setTimezone, theme, setTheme, motionPaused, setMotionPaused],
  );
  return <PrefsContext.Provider value={value}>{children}</PrefsContext.Provider>;
}

export function usePrefs(): Prefs {
  const ctx = useContext(PrefsContext);
  if (!ctx) throw new Error('usePrefs must be used within PrefsProvider');
  return ctx;
}

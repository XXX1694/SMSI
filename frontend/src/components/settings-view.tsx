'use client';
import { useState } from 'react';
import { useAuth } from '@/components/auth-provider';
import { usePrefs, type Theme } from '@/components/prefs-provider';
import { PasswordForm } from '@/components/password-form';
import { Section } from '@/components/ui/card';
import { Field, Input, Select } from '@/components/ui/input';
import { browserTimezone, formatDateTime, isValidTimezone } from '@/lib/time';

function tzOptions(current: string): string[] {
  let list: string[] = [];
  try {
    list = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf?.('timeZone') ?? [];
  } catch {
    list = [];
  }
  const set = new Set(['UTC', browserTimezone(), current, ...list]);
  return [...set].filter(isValidTimezone).sort();
}

export function SettingsView() {
  const { user } = useAuth();
  const { timezone, setTimezone, theme, setTheme } = usePrefs();
  const [zones] = useState(() => tzOptions(timezone));

  return (
    <div className="max-w-xl space-y-10">
      <Section title="Profile">
        <dl className="grid grid-cols-[8rem_1fr] gap-y-2 text-sm">
          <dt className="text-muted-foreground">Name</dt>
          <dd>{user?.display_name}</dd>
          <dt className="text-muted-foreground">Email</dt>
          <dd>
            {user?.email}
            {user && user.verification_enforced ? (
              <span className="ml-2 text-xs text-muted-foreground">{user.email_verified ? 'verified' : 'not verified'}</span>
            ) : null}
          </dd>
        </dl>
      </Section>
      <Section title="Preferences">
        <div className="space-y-4">
          <Field label="Timezone" hint={`Scheduled times are entered in this zone and stored as UTC. Now: ${formatDateTime(new Date().toISOString(), timezone)}.`}>
            <Select value={timezone} onChange={(e) => setTimezone(e.target.value)}>
              {zones.map((z) => (
                <option key={z} value={z}>
                  {z}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Theme">
            <Select value={theme} onChange={(e) => setTheme(e.target.value as Theme)}>
              <option value="system">System</option>
              <option value="light">Light</option>
              <option value="dark">Dark</option>
            </Select>
          </Field>
        </div>
      </Section>
      <Section title="Security">
        <Field label="Session" hint="You are signed in with a secure session cookie. Sign out from the sidebar.">
          <Input value="Active" readOnly disabled />
        </Field>
      </Section>
      <Section title="Password">
        <PasswordForm />
      </Section>
    </div>
  );
}

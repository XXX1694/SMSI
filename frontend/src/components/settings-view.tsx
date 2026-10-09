'use client';
import Link from 'next/link';
import { useTranslations } from '@/i18n/use-translations';
import { useState } from 'react';
import { useAuth } from '@/components/auth-provider';
import { usePrefs, type Theme } from '@/components/prefs-provider';
import { DataExportCard } from '@/components/data-export';
import { PasswordForm } from '@/components/password-form';
import { Section } from '@/components/ui/card';
import { UsageCard } from '@/components/usage-card';
import { Field, Select } from '@/components/ui/input';
import { LanguageSelect } from '@/i18n/language-select';
import { useFormat } from '@/i18n/use-format';
import { browserTimezone, isValidTimezone } from '@/lib/time';

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
  const tl = useTranslations('language');
  const fmt = useFormat();

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
      <Section title="Plan & usage">
        <UsageCard />
      </Section>
      <Section title="Preferences">
        <div className="space-y-4">
          <Field label="Time zone" hint={`Times you enter and see use this time zone. Now: ${fmt.dateTime(new Date())}.`}>
            <Select value={timezone} onChange={(e) => setTimezone(e.target.value)}>
              {zones.map((z) => (
                <option key={z} value={z}>
                  {z}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={tl('label')} hint={tl('hint')}>
            <LanguageSelect />
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
      <Section title="Password">
        <PasswordForm />
      </Section>
      <Section title="Your data">
        <DataExportCard />
      </Section>
      <Section title="Legal">
        <p className="text-sm text-muted-foreground">
          Read the{' '}
          <Link href="/terms" className="text-accent underline underline-offset-4 hover:no-underline">
            Terms of Service
          </Link>{' '}
          and the{' '}
          <Link href="/privacy" className="text-accent underline underline-offset-4 hover:no-underline">
            Privacy Policy
          </Link>
          . The server admin is responsible for both.
        </p>
      </Section>
    </div>
  );
}

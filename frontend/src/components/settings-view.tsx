'use client';
import Link from 'next/link';
import { useTranslations } from '@/i18n/use-translations';
import { useState } from 'react';
import { useAuth } from '@/components/auth-provider';
import { usePrefs, type Theme } from '@/components/prefs-provider';
import { PasswordForm } from '@/components/password-form';
import { Section } from '@/components/ui/card';
import { YourData } from '@/components/your-data';
import { UsageCard } from '@/components/usage-card';
import { CheckboxField } from '@/components/ui/checkbox';
import { Field, Select } from '@/components/ui/input';
import { LanguageSelect } from '@/i18n/language-select';
import { useFormat } from '@/i18n/use-format';
import { browserTimezone, isValidTimezone } from '@/lib/time';
import { nodes } from '@/i18n/rich';

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
  const { timezone, setTimezone, theme, setTheme, motionPaused, setMotionPaused } = usePrefs();
  const [zones] = useState(() => tzOptions(timezone));
  const tl = useTranslations('language');
  const t = useTranslations('settings');
  const fmt = useFormat();

  return (
    <div className="max-w-xl space-y-10">
      <Section title={t('profile')}>
        <dl className="grid grid-cols-[8rem_1fr] gap-y-2 text-sm">
          <dt className="text-muted-foreground">{t('name')}</dt>
          <dd>{user?.display_name}</dd>
          <dt className="text-muted-foreground">{t('email')}</dt>
          <dd>
            {user?.email}
            {user && user.verification_enforced ? (
              <span className="ml-2 text-xs text-muted-foreground">{user.email_verified ? t('verified') : t('notVerified')}</span>
            ) : null}
          </dd>
        </dl>
      </Section>
      <Section title={t('planUsage')}>
        <UsageCard />
      </Section>
      <Section title={t('preferences')}>
        <div className="space-y-4">
          <Field label={t('timeZone')} hint={t('timeZoneHint', { now: fmt.dateTime(new Date()) })}>
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
          <Field label={t('theme')}>
            <Select value={theme} onChange={(e) => setTheme(e.target.value as Theme)}>
              <option value="system">{t('themeSystem')}</option>
              <option value="light">{t('themeLight')}</option>
              <option value="dark">{t('themeDark')}</option>
            </Select>
          </Field>
          <CheckboxField id="pause-motion" label={t('pauseMotion')} description={t('pauseMotionHint')} checked={motionPaused} onCheckedChange={(c) => setMotionPaused(c === true)} />
        </div>
      </Section>
      <Section title={t('password')}>
        <PasswordForm />
      </Section>
      <YourData />
      <Section title={t('legal')}>
        <p className="text-sm text-muted-foreground">
          {nodes(
            t.rich('legalText', {
              terms: (c) => (
                <Link href="/terms" className="text-accent underline underline-offset-4 hover:no-underline">
                  {c}
                </Link>
              ),
              privacy: (c) => (
                <Link href="/privacy" className="text-accent underline underline-offset-4 hover:no-underline">
                  {c}
                </Link>
              ),
            }),
          )}
        </p>
      </Section>
    </div>
  );
}

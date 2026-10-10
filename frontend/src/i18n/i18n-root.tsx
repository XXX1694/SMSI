'use client';
import type { ComponentProps } from 'react';
import { LegacyMessagesScope } from '@/i18n/legacy-scope';
import { LocaleProvider } from '@/i18n/locale-provider';

/**
 * What the root layout and the tests that need real text render: the locale provider with the one legacy scope that holds
 * every bundle (#145, step 2). One component, so a test cannot pass on a tree the app does not run.
 */
export function I18nRoot({ children, ...props }: ComponentProps<typeof LocaleProvider>) {
  return (
    <LocaleProvider {...props}>
      <LegacyMessagesScope>{children}</LegacyMessagesScope>
    </LocaleProvider>
  );
}

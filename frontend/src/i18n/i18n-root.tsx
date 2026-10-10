'use client';
import type { ComponentProps } from 'react';
import { LocaleProvider } from '@/i18n/locale-provider';
import { CoreScope } from '@/i18n/scopes/core';

/**
 * What the root layout and the tests that need real text render: the locale provider and the bundles every route needs
 * (`common`, `errors`). Routes add their own with the scopes in src/i18n/scopes (see docs/copy/translation-process.md).
 * One component, so a test cannot pass on a tree the app does not run.
 */
export function I18nRoot({ children, ...props }: ComponentProps<typeof LocaleProvider>) {
  return (
    <LocaleProvider {...props}>
      <CoreScope>{children}</CoreScope>
    </LocaleProvider>
  );
}

import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

const auth = vi.hoisted(() => ({ user: null as unknown }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => auth }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC', setTimezone: vi.fn(), theme: 'system', setTheme: vi.fn() }) }));
vi.mock('@/components/sign-in-methods', () => ({ SignInMethods: () => <div>sign-in methods</div> }));
vi.mock('@/components/password-form', () => ({ PasswordForm: () => <form aria-label="change password form" /> }));
vi.mock('@/components/link-result-notice', () => ({ LinkResultNotice: () => null }));
vi.mock('@/components/usage-card', () => ({ UsageCard: () => null }));
vi.mock('@/components/your-data', () => ({ YourData: () => null }));
vi.mock('@/i18n/language-select', () => ({ LanguageSelect: () => null }));

import { SettingsView } from '@/components/settings-view';

const user = (has_password: boolean) => ({ display_name: 'Sam', email: 's@example.com', verification_enforced: false, email_verified: true, has_password });

describe('SettingsView password section', () => {
  it('offers changing the password to accounts that have one', () => {
    auth.user = user(true);
    render(<SettingsView />);
    expect(screen.getByRole('heading', { name: 'Password' })).toBeInTheDocument();
    expect(screen.getByRole('form', { name: 'change password form' })).toBeInTheDocument();
    expect(screen.getByText('sign-in methods')).toBeInTheDocument();
  });

  it('hides it for accounts without one: their first password is set under Sign-in methods', () => {
    auth.user = user(false);
    render(<SettingsView />);
    expect(screen.queryByRole('heading', { name: 'Password' })).toBeNull();
    expect(screen.queryByRole('form', { name: 'change password form' })).toBeNull();
    expect(screen.getByRole('heading', { name: 'Sign-in methods' })).toBeInTheDocument();
  });
});

import { render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AppShell } from '@/components/app-shell';
import { PrefsProvider } from '@/components/prefs-provider';
import { I18nRoot } from '@/i18n/i18n-root';

vi.mock('next/navigation', () => ({ usePathname: () => '/posts', useRouter: () => ({ replace: vi.fn(), push: vi.fn() }) }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => ({ user: { email: 'a@example.com', display_name: 'Ana' }, logout: vi.fn() }) }));
vi.mock('@/components/approvals/use-pending-approvals', () => ({ usePendingApprovals: () => 0 }));
vi.mock('@/components/email-banner', () => ({ EmailBanner: () => null }));
vi.mock('@/components/page-transition', () => ({ PageTransition: ({ children }: { children: React.ReactNode }) => <>{children}</> }));

const ENABLED = ['en'] as const;

function shell() {
  return render(
    <PrefsProvider>
      <I18nRoot enabled={ENABLED}>
        <AppShell>content</AppShell>
      </I18nRoot>
    </PrefsProvider>,
  );
}

describe('sidebar navigation labels come from the catalog', () => {
  beforeEach(() => window.localStorage.clear());

  it('shows the English labels', () => {
    shell();
    const nav = screen.getByRole('navigation', { name: 'Main' });
    for (const label of ['Dashboard', 'Compose', 'Posts', 'Calendar', 'Media', 'Analytics', 'Accounts', 'Approvals']) {
      expect(within(nav).getByRole('link', { name: label })).toBeInTheDocument();
    }
    expect(screen.getByRole('link', { name: 'Settings' })).toBeInTheDocument();
  });

  it('shows pseudo-locale labels when en-XA is chosen (proves the pipeline end to end)', async () => {
    window.localStorage.setItem('steerpost_locale', 'en-XA');
    shell();
    await waitFor(() => expect(screen.getByRole('link', { name: /^\[Ďààšĥ/ })).toBeInTheDocument());
    expect(screen.queryByRole('link', { name: 'Dashboard' })).not.toBeInTheDocument();
  });

  it('offers the compact language switcher only when there is a choice (en + pseudo here)', () => {
    shell();
    expect(screen.getByRole('combobox', { name: 'Language' })).toBeInTheDocument();
  });
});

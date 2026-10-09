import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

vi.mock('next/navigation', () => ({ usePathname: () => '/dashboard', useRouter: () => ({ replace: vi.fn(), push: vi.fn() }) }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => ({ user: { email: 'a@b.c', display_name: 'A', email_verified: true }, logout: vi.fn() }) }));
vi.mock('@/components/approvals/use-pending-approvals', () => ({ usePendingApprovals: () => 3 }));
vi.mock('@/i18n/locale-provider', async () => ({ ...(await vi.importActual<typeof import('@/i18n/locale-provider')>('@/i18n/locale-provider')), useLocaleSettings: () => ({ available: ['en'] }) }));
vi.mock('@/components/email-banner', () => ({ EmailBanner: () => null }));

import { AppShell } from '@/components/app-shell';

describe('AppShell keyboard and touch access', () => {
  it('starts with a skip link that targets the focusable main landmark', () => {
    render(<AppShell>content</AppShell>);
    const skip = screen.getByRole('link', { name: 'Skip to content' });
    expect(skip).toHaveAttribute('href', '#main');
    expect(document.body.querySelector('a, button')).toBe(skip); // first tab stop
    const main = screen.getByRole('main');
    expect(main).toHaveAttribute('id', 'main');
    expect(main).toHaveAttribute('tabindex', '-1');
  });

  it('closes the mobile menu on Escape and returns focus to the menu button', async () => {
    render(<AppShell>content</AppShell>);
    const toggle = screen.getByRole('button', { name: 'Open menu' });
    await userEvent.click(toggle);
    expect(toggle).toHaveAccessibleName('Close menu');
    await userEvent.keyboard('{Escape}');
    expect(toggle).toHaveAccessibleName('Open menu');
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(toggle).toHaveFocus();
  });
});

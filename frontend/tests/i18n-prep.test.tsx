import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { enT } from './helpers/en-t';
import { postPlatforms, providerName } from '@/lib/format';
import { summaryLines } from '@/lib/approvals';
import type { Approval, Post } from '@/lib/types';

const pending = vi.hoisted(() => ({ n: 150 }));
vi.mock('next/navigation', () => ({ usePathname: () => '/dashboard', useRouter: () => ({ replace: vi.fn(), push: vi.fn() }) }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => ({ user: { email: 'a@b.c', display_name: 'A', email_verified: true }, logout: vi.fn() }) }));
vi.mock('@/components/approvals/use-pending-approvals', () => ({ usePendingApprovals: () => pending.n }));
vi.mock('@/i18n/locale-provider', async () => {
  const { FALLBACK } = await vi.importActual<typeof import('@/i18n/locale-context')>('@/i18n/locale-context');
  return { ...(await vi.importActual<typeof import('@/i18n/locale-provider')>('@/i18n/locale-provider')), useLocaleSettings: () => ({ ...FALLBACK, available: ['en'] }) };
});
vi.mock('@/components/email-banner', () => ({ EmailBanner: () => null }));

import { AppShell } from '@/components/app-shell';

describe('approvals badge', () => {
  it('shows 99+ but tells screen readers the real count in one sentence', () => {
    pending.n = 150;
    render(<AppShell>x</AppShell>);
    expect(screen.getAllByText('99+').length).toBeGreaterThan(0);
    expect(screen.getAllByText('150 requests waiting').length).toBeGreaterThan(0);
  });
  it('uses the singular and the plain number below 100', () => {
    pending.n = 1;
    render(<AppShell>x</AppShell>);
    expect(screen.getAllByText('1 request waiting').length).toBeGreaterThan(0);
  });
  it('the count-only mobile pill carries the Approvals icon, hidden from assistive tech', () => {
    pending.n = 2;
    const { container } = render(<AppShell>x</AppShell>);
    const pill = container.querySelector('header a[href="/approvals"]');
    const icon = pill?.querySelector('svg');
    expect(icon).not.toBeNull();
    expect(icon?.closest('[aria-hidden="true"]')).not.toBeNull();
  });
});

describe('the catalog messages that used to be glued together', () => {
  it('states the capability in one sentence per state', () => {
    expect(enT('accounts.caps.supports', { capability: 'video' })).toBe('Supports video posts');
    expect(enT('accounts.caps.lacks', { capability: 'schedule' })).toBe('Does not support scheduling by the network');
  });
  it('words the calendar day link as one plural sentence with a formatted date', () => {
    expect(enT('calendar.moreOnDay', { count: 1, day: 'Monday 12 October' })).toBe('1 more post on Monday 12 October, open the day');
    expect(enT('calendar.postsOnDay', { count: 3, day: 'Monday 12 October' })).toBe('3 posts on Monday 12 October, open the day');
  });
  it('picks the locked-post sentence from the status code, not from a label', () => {
    expect(enT('composer.conflictLocked', { status: 'published' })).toBe('It is now published and cannot be edited.');
    expect(enT('composer.conflictLocked', { status: 'partially_published' })).toBe('It is now partially published and cannot be edited.');
    expect(enT('composer.conflictLocked', { status: 'cancelled' })).toBe('It is now canceled and cannot be edited.');
    expect(enT('composer.conflictLocked', { status: 'weird' })).toBe('It is now in a state that cannot be edited.');
  });
});

describe('an empty network id reads "Unknown"', () => {
  it('providerName falls back to the catalog', () => {
    expect(providerName('', enT)).toBe('Unknown');
    expect(providerName('linkedin', enT)).toBe('LinkedIn');
    expect(providerName('newnet', enT)).toBe('Newnet');
  });
  it('a post whose target has no platform is not shown as an empty list', () => {
    const post = { targets: [{ platform: '' }] } as unknown as Pick<Post, 'targets'>;
    expect(postPlatforms(post, enT)).toBe('Unknown');
    expect(postPlatforms({ targets: [] }, enT)).toBe('No targets');
  });
  it('an approval text line for a target with no account or platform does not end in "on "', () => {
    const a = { summary: { targets: [{ platform: '', content: 'hi' }] } } as unknown as Approval;
    expect(summaryLines(a, 'UTC', enT)).toEqual([{ label: 'Text on Unknown', value: 'hi', long: false }]);
  });
});

import { render } from '@testing-library/react';
import { afterAll, beforeAll } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it, vi } from 'vitest';
import { ApprovalCard } from '@/components/approvals/approval-card';
import { AuthForm } from '@/components/auth-form';
import { CapabilityBadges } from '@/components/capability-badges';
import { EmptyState, ErrorState } from '@/components/states';
import { ForgotPasswordView } from '@/components/forgot-password-view';
import { PostRow } from '@/components/post-row';
import { AccountsView } from '@/components/accounts-view';
import { CalendarViewPage } from '@/components/calendar-view';
import { ComposerView } from '@/components/composer/composer-view';
import { DashboardView } from '@/components/dashboard-view';
import { ApiKeysView } from '@/components/developer/api-keys-view';
import { McpView } from '@/components/developer/mcp-view';
import { UsageView } from '@/components/developer/usage-view';
import { PostDetail } from '@/components/posts/post-detail';
import { ToastProvider } from '@/components/toast';
import { SettingsView } from '@/components/settings-view';
import { SEEDED_POST_IDS } from '@/lib/demo/ids';
import { buildSeed } from '@/lib/demo/seed';
import { PrefsProvider } from '@/components/prefs-provider';
import { ScopePicker } from '@/components/developer/scope-picker';
import { TrustedPolicyField } from '@/components/developer/trusted-policy';
import { LocaleProvider } from '@/i18n/locale-provider';
import { createTranslator } from '@/i18n/translate';
import { enT } from '@/i18n/en';
import { ApiError } from '@/lib/api';
import { errorMessage } from '@/hooks';
import { editBlockedReason } from '@/lib/status';
import { formatBytes } from '@/lib/media';
import { formatRelative } from '@/lib/time';
import type { Approval, Post } from '@/lib/types';
import en from '../messages/en.json';
import meta from '../messages/meta.json';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => '/',
}));
const authState = vi.hoisted(() => ({ user: null as null | Record<string, unknown> }));
vi.mock('@/components/auth-provider', () => ({ useAuth: () => ({ login: vi.fn(), register: vi.fn(), user: authState.user, refresh: vi.fn() }) }));

const tFor = (locale: string) => createTranslator({ locale, messages: en, onMissing: () => {} });

describe('counts use ICU plurals, never "1 posts"', () => {
  const t = enT;
  it.each([
    ['shell.requestsWaiting', 1, '1 request waits for you'],
    ['shell.requestsWaiting', 3, '3 requests wait for you'],
    ['composer.fixProblems', 1, 'Fix 1 problem first'],
    ['composer.fixProblems', 2, 'Fix 2 problems first'],
    ['accounts.orphans', 1, '1 account belongs to a network that is no longer available.'],
    ['accounts.orphans', 2, '2 accounts belong to a network that is no longer available.'],
    ['calendar.postCount', 1, '1 post'],
    ['calendar.postCount', 4, '4 posts'],
    ['calendar.more', 1, '+1 more'],
    ['calendar.more', 1200, '+1,200 more'],
    ['media.attach', 1, 'Attach 1 file'],
    ['media.attach', 3, 'Attach 3 files'],
    ['approvals.minLeft', 1, '1 min left'],
    ['approvals.minLeft', 9, '9 min left'],
  ] as const)('%s with %i', (key, count, text) => {
    expect(t(key, { count })).toBe(text);
  });

  it('plural and select messages cover state and count together', () => {
    expect(t('posts.targetMeta', { hasDate: 'true', when: '9 Oct 2026, 14:30', count: 1 })).toBe('Published 9 Oct 2026, 14:30 · 1 attempt');
    expect(t('posts.targetMeta', { hasDate: 'false', when: '', count: 3 })).toBe('3 attempts');
    expect(t('composer.counter', { label: 'X', len: 5, max: 1 })).toBe('X: 5 of 1 character');
    expect(t('composer.counter', { label: 'X', len: 5, max: 1000 })).toBe('X: 5 of 1,000 characters');
    expect(t('composer.v.mediaMax', { account: 'A', network: 'X', max: 1 })).toBe('A: X allows 1 attachment at most.');
    expect(t('composer.v.overLimit', { account: 'A', over: 1, network: 'X', limit: 10 })).toBe('A: 1 character over the X limit of 10.');
  });
});

describe('editBlockedReason passes the API status code to the select', () => {
  it('uses the real catalog', () => {
    expect(editBlockedReason('failed', enT)).toBe('Only drafts and scheduled posts can be edited. This post is failed.');
    expect(editBlockedReason('partially_published', enT)).toBe('Only drafts and scheduled posts can be edited. This post is partially published.');
    expect(editBlockedReason('canceled' as never, enT)).toContain('in a state that cannot be edited');
    expect(editBlockedReason('cancelled', enT)).toBe('This post was canceled and cannot be edited.');
  });
  it('picks the branch by status code even when every branch reads differently', () => {
    const messages = { posts: { editBlockedOther: '{status, select, failed {BRANCH-F} partially_published {BRANCH-P} other {BRANCH-O}}' } };
    const t = createTranslator({ locale: 'en', messages: messages as never, onMissing: () => {} });
    expect(editBlockedReason('failed', t as never)).toBe('BRANCH-F');
    expect(editBlockedReason('partially_published', t as never)).toBe('BRANCH-P');
  });
});

describe('errors outside English never show the server text', () => {
  const server = new ApiError(400, 'VALIDATION_ERROR', 'Email already in use.', 'req-1');
  it('English keeps the readable server sentence and appends the reference', () => {
    expect(errorMessage(server, tFor('en') as never)).toBe('Email already in use. (ref req-1)');
  });
  it('another language shows the sentence for the code, still with the reference', () => {
    expect(errorMessage(server, tFor('ru') as never)).toBe('Some of the details are not valid. Check them and try again. (ref req-1)');
  });
  it('the pseudo-locale counts as English (the server text is shown)', () => {
    expect(errorMessage(server, tFor('en-XA') as never, false)).toBe('Email already in use.');
  });
});

describe('numbers, sizes and times follow the locale', () => {
  it('formats sizes with the locale decimal separator', () => {
    expect(formatBytes(1.5 * 1024 * 1024, enT)).toBe('1.5 MB');
    expect(formatBytes(2048 * 1024 * 1024, enT)).toBe('2,048 MB');
    expect(formatBytes(1.5 * 1024 * 1024, tFor('de'))).toBe('1,5 MB');
  });
  it('formats relative times in the locale', () => {
    const now = new Date('2026-01-01T12:00:00Z');
    expect(formatRelative('2026-01-01T10:00:00Z', tFor('de'), now)).toBe('vor 2 Stunden');
  });
});

describe('catalog notes for translators', () => {
  const keys = Object.keys(meta) as (keyof typeof meta)[];
  it('every English key has a description', () => {
    const flat = (o: object, p = ''): string[] => Object.entries(o).flatMap(([k, v]) => (typeof v === 'string' ? [`${p}${k}`] : flat(v, `${p}${k}.`)));
    const missing = flat(en).filter((k) => !(k in meta) || !(meta as Record<string, { description?: string }>)[k]?.description);
    expect(missing).toEqual([]);
    expect(keys.length).toBeGreaterThan(500);
  });
  it.each([
    ['common.status.post.draft', /noun/],
    ['composer.schedule', /noun/],
    ['composer.scheduleAction', /verb/],
    ['accounts.telegram.step2', /verb/],
    ['composer.publishTo', /object/],
    ['accounts.connect', /object/],
    ['approvals.status.expired', /Keep separate keys/],
    ['common.status.account.revoked', /API key/],
    ['common.never', /never happened/],
    ['developer.apiKeys.expiryNever', /does not expire/],
    ['calendar.previous', /date period/],
    ['calendar.next', /date period/],
    ['calendar.today', /current date/],
    ['approvals.minLeft', /plural/],
    ['common.cancel', /Not the post action/],
    ['posts.cancelPost', /Not the dialog button/],
    ['posts.keepPost', /Dismiss/],
    ['common.tryAgain', /Never merge/],
    ['common.retry', /Not "Try again"/],
    ['approvals.requestedBy', /Do not inflect/],
    ['approvals.approvedToast', /shown as a name/],
  ] as const)('%s carries its context note', (key, pattern) => {
    expect((meta as Record<string, { description: string }>)[key]?.description).toMatch(pattern);
  });
  it('the source tree still has no translator-note comments left behind', () => {
    const root = join(__dirname, '..', 'src');
    const files = ['components/confirm-dialog.tsx', 'components/accounts-view.tsx', 'components/states.tsx', 'lib/status.ts', 'lib/approvals.ts'];
    for (const f of files) expect(readFileSync(join(root, f), 'utf8')).not.toMatch(/Translator note/);
  });
});

/** Text a person reads or hears: text nodes and the attributes that carry copy. */
function visibleStrings(root: HTMLElement): string[] {
  const out: string[] = [];
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const text = (n.textContent ?? '').trim();
    if (text) out.push(text);
  }
  for (const el of root.querySelectorAll('[aria-label],[title],[placeholder],[alt]')) {
    for (const a of ['aria-label', 'title', 'placeholder', 'alt']) {
      const v = el.getAttribute(a);
      if (v) out.push(v);
    }
  }
  return out;
}

/** In en-XA every catalog message is accented and bracketed, so plain English that is left over is a hard-coded string. */
function expectNoPlainEnglish(root: HTMLElement, allowed: (RegExp | ((s: string) => boolean))[] = []) {
  const ok = (s: string) => allowed.some((a) => (typeof a === 'function' ? a(s) : a.test(s)));
  const plain = visibleStrings(root).filter((s) => /[A-Za-z]{3,}/.test(s) && !/[À-ɏ]/.test(s) && !ok(s));
  expect(plain).toEqual([]);
}

function inPseudo(ui: React.ReactElement) {
  window.localStorage.setItem('steerpost_locale', 'en-XA');
  return render(
    <PrefsProvider>
      <LocaleProvider enabled={['en']}>
        <ToastProvider>{ui}</ToastProvider>
      </LocaleProvider>
    </PrefsProvider>,
  );
}

const approval: Approval = {
  id: 'ap1', action: 'post.publish', resource_type: 'post', resource_id: 'p1', actor_label: 'Claude Desktop', status: 'pending',
  summary: { title: 'Launch', content: 'x'.repeat(400), platforms: ['linkedin'], media: { images: 2 }, targets: [{ platform: 'linkedin', content: 'hi' }] },
  created_at: '2026-10-08T11:58:00Z', expires_at: new Date(Date.now() + 8 * 60_000).toISOString(), decided_at: null,
};
const post = {
  id: 'p1', status: 'failed', title: '', content: '', scheduled_at: null, published_at: null, created_at: '2026-10-01T10:00:00Z',
  targets: [{ id: 't1', platform: 'telegram', status: 'failed', content: '' }],
} as unknown as Post;

describe('no plain English is left on screen in the pseudo-locale', () => {
  /** The catalog loads after mount; wait until the pseudo text is on screen. */
  async function settle(root: HTMLElement) {
    await vi.waitFor(() => expect(root.textContent ?? '').toMatch(/[\u00C0-\u024F]/));
  }
  it('sign-in and registration forms', async () => {
    for (const mode of ['login', 'register'] as const) {
      const { container, unmount } = inPseudo(<AuthForm mode={mode} />);
      await settle(container);
      // "Steerpost" in the title is part of an accented message; the demo credentials are not shown here.
      expectNoPlainEnglish(container);
      unmount();
    }
  });
  it('forgot password', async () => {
    const { container } = inPseudo(<ForgotPasswordView />);
    await settle(container);
    expectNoPlainEnglish(container);
  });
  it('states, errors and badges', async () => {
    const { container } = inPseudo(
      <>
        <ErrorState error={new ApiError(500, 'INTERNAL', 'INTERNAL')} onRetry={() => {}} />
        <EmptyState title="x">y</EmptyState>
        <CapabilityBadges caps={{ canPublishText: true, canPublishImage: false, canPublishVideo: false, canSchedule: false, canDelete: true, canAnalytics: false, maxTextLength: 3000 } as never} />
      </>,
    );
    await settle(container);
    expectNoPlainEnglish(container, [/^[xy]$/]);
  });
  it('an approval card, a post row, the scope picker and the trusted-key field', async () => {
    const { container } = inPseudo(
      <>
        <ul>
          <ApprovalCard approval={approval} now={new Date()} busy={false} onApprove={() => {}} onDeny={() => {}} />
          <ApprovalCard approval={{ ...approval, status: 'consumed', decided_at: '2026-10-08T12:00:00Z' }} now={new Date()} busy={false} onApprove={() => {}} onDeny={() => {}} />
        </ul>
        <PostRow post={post} onRetry={() => {}} />
        <ScopePicker value={['posts:publish']} onChange={() => {}} />
        <TrustedPolicyField trusted confirmed={false} onTrusted={() => {}} onConfirmed={() => {}} />
      </>,
    );
    await settle(container);
    // Scope ids (`posts:publish`), the agent name, the network name, the post title and the dates (Intl month names) are data, not copy.
    expectNoPlainEnglish(container, [/^[a-z]+:[a-z]+$/, /^Claude Desktop$/, /^(LinkedIn|Telegram)$/, /^x+$/, /^Launch$/, /^\d{1,2} \w{3} \d{4}, \d{2}:\d{2}$/]);
  });
});

describe('the app screens are fully translated in the pseudo-locale (demo data, no network)', () => {
  const seed = JSON.stringify(buildSeed(new Date()));
  /** Names, titles, post text, key names and other data the user typed or the server sent are not catalog copy. */
  const isSeedData = (s: string) => seed.includes(s) || s.split(/\s·\s|\n/).every((part) => seed.includes(part.trim()));
  const data: (RegExp | ((s: string) => boolean))[] = [
    isSeedData,
    /^[a-z]+:[a-z]+(, [a-z]+:[a-z]+)*$/,
    /^(LinkedIn|Telegram|Test network|Claude Desktop|UTC|free|English|Discord|Mastodon|Bluesky|Instagram|Facebook|TikTok|YouTube|Threads|Pinterest)$/,
    /^(LinkedIn|Telegram|Test network)(, (LinkedIn|Telegram|Test network))*$/, // network names joined with the catalog separator
    /^[A-Z][A-Za-z_-]+\/[A-Za-z_/-]+$/, // time zone ids
    /^\d{1,2} \w{3,4} \d{4}, \d{2}:\d{2}$/,
    /^(\d+ \w+ ago|yesterday|tomorrow|in \d+ \w+)$/, // Intl relative times: the pseudo-locale formats like en
    /^Authorization: Bearer /, // a code sample
    /^(Personal profile only|Add the Steerpost bot|Test network\.|Posts to one channel|Mastodon and|Text up to|Not available yet)/, // provider notes: the server sends them in English (D-021)
    /^(Mon|Tue|Wed|Thu|Fri|Sat|Sun)\w* ?\d*$/, // weekday names come from Intl, in the locale
    /^(January|February|March|April|May|June|July|August|September|October|November|December)( \d{4})?$/,
  ];
  beforeAll(() => {
    vi.stubEnv('NEXT_PUBLIC_DEMO', 'true');
    authState.user = { id: 'u1', email: 'demo@example.com', display_name: 'Demo', email_verified: true, verification_enforced: false };
  });
  afterAll(() => {
    vi.unstubAllEnvs();
    authState.user = null;
  });
  async function check(ui: React.ReactElement, loaded: RegExp) {
    const { container, unmount } = inPseudo(ui);
    await vi.waitFor(() => expect(container.textContent ?? '').toMatch(loaded), { timeout: 4000 });
    expectNoPlainEnglish(container, data);
    unmount();
  }
  const accented = /[\u00C0-\u024F]{3,}/;
  it('dashboard', async () => check(<DashboardView />, /Failed|Ƒ|Ŕéçéñţ/));
  it('calendar', async () => check(<CalendarViewPage />, accented));
  it('accounts', async () => check(<AccountsView />, accented));
  it('settings', async () => check(<SettingsView />, accented));
  it('developer: API keys, MCP connections and usage', async () => {
    await check(<ApiKeysView />, accented);
    await check(<McpView />, accented);
    await check(<UsageView />, accented);
  });
  it('composer', async () => check(<ComposerView />, accented));
  it('post detail', async () => check(<PostDetail id={SEEDED_POST_IDS[0] ?? ''} />, accented));
});

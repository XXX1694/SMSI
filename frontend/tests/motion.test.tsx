import { act, fireEvent, render, screen } from '@testing-library/react';
import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TransitionLink, hrefToString } from '@/components/transition-link';
import { isCurrentLocation, notifyNavigated, runWithTransition } from '@/lib/view-transition';

const push = vi.fn();
vi.mock('next/navigation', () => ({ useRouter: () => ({ push }), usePathname: () => '/' }));

function mockReducedMotion(reduce: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: reduce && q.includes('reduce'), media: q, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia;
}
const doc = document as unknown as { startViewTransition?: unknown };

beforeEach(() => {
  push.mockReset();
  window.history.pushState({}, '', '/');
  mockReducedMotion(false);
});
afterEach(() => {
  delete doc.startViewTransition;
  vi.useRealTimers();
});

describe('runWithTransition', () => {
  it('navigates directly when the API is missing', () => {
    const nav = vi.fn();
    expect(runWithTransition(nav)).toBe(false);
    expect(nav).toHaveBeenCalledTimes(1);
  });

  it('skips the transition under prefers-reduced-motion', () => {
    const start = vi.fn();
    doc.startViewTransition = start;
    mockReducedMotion(true);
    const nav = vi.fn();
    expect(runWithTransition(nav)).toBe(false);
    expect(start).not.toHaveBeenCalled();
    expect(nav).toHaveBeenCalledTimes(1);
  });

  it('runs inside a view transition and settles once the navigation is committed', async () => {
    let settled = false;
    doc.startViewTransition = vi.fn((update: () => Promise<void>) => {
      void update().then(() => (settled = true));
    });
    const nav = vi.fn();
    expect(runWithTransition(nav)).toBe(true);
    expect(nav).toHaveBeenCalledTimes(1);
    expect(settled).toBe(false);
    notifyNavigated();
    await new Promise((r) => setTimeout(r, 5));
    expect(settled).toBe(true);
  });

  it('does not hang when the navigation never commits', async () => {
    vi.useFakeTimers();
    let settled = false;
    doc.startViewTransition = vi.fn((update: () => Promise<void>) => {
      void update().then(() => (settled = true));
    });
    runWithTransition(() => {});
    await vi.advanceTimersByTimeAsync(1100);
    expect(settled).toBe(true);
  });
});

describe('TransitionLink', () => {
  it('pushes through the router on a plain click', () => {
    render(<TransitionLink href="/posts">Posts</TransitionLink>);
    fireEvent.click(screen.getByRole('link', { name: 'Posts' }));
    expect(push).toHaveBeenCalledWith('/posts');
  });

  it('leaves modified clicks to the browser', () => {
    render(<TransitionLink href="/posts">Posts</TransitionLink>);
    fireEvent.click(screen.getByRole('link', { name: 'Posts' }), { metaKey: true });
    expect(push).not.toHaveBeenCalled();
  });
});

describe('same-page clicks', () => {
  it('detects the current location ignoring trailing slashes, and compares query and hash', () => {
    window.history.pushState({}, '', '/posts/?status=draft#top');
    expect(isCurrentLocation('/posts?status=draft#top')).toBe(true);
    expect(isCurrentLocation('/posts')).toBe(false);
    expect(isCurrentLocation('/posts?status=failed#top')).toBe(false);
    expect(isCurrentLocation('/compose?status=draft#top')).toBe(false);
  });

  it('does not start a transition or push when the target is the current page', () => {
    const start = vi.fn();
    doc.startViewTransition = start;
    window.history.pushState({}, '', '/posts');
    render(<TransitionLink href="/posts">Posts</TransitionLink>);
    const link = screen.getByRole('link', { name: 'Posts' });
    // The click is left to the browser/Next default: not prevented by us, no transition, no extra push.
    expect(fireEvent.click(link)).toBe(true);
    expect(start).not.toHaveBeenCalled();
    expect(push).not.toHaveBeenCalled();
  });

  it('keeps query and hash of an object href', () => {
    expect(hrefToString({ pathname: '/posts', query: { status: 'draft' }, hash: 'x' })).toBe('/posts?status=draft#x');
    expect(hrefToString({ pathname: '/posts', search: '?a=1' })).toBe('/posts?a=1');
    window.history.pushState({}, '', '/');
    render(<TransitionLink href={{ pathname: '/posts', query: { status: 'draft' } }}>Drafts</TransitionLink>);
    fireEvent.click(screen.getByRole('link', { name: 'Drafts' }));
    expect(push).toHaveBeenCalledWith('/posts?status=draft');
  });
});

describe('stylesheet guards', () => {
  const css = readFileSync(`${__dirname}/../src/app/globals.css`, 'utf8');
  it('stagger does not keep a forward fill that would pin transform over :hover', () => {
    expect(css).toMatch(/\.stagger > \* \{[^}]*backwards/);
    expect(css).not.toMatch(/\.stagger > \* \{[^}]*\bboth\b/);
  });
  it('defines the keyframes it uses by name and does not cross-fade the shell', () => {
    for (const name of ['rise-in', 'fade-out', 'shimmer']) expect(css).toContain(`@keyframes ${name}`);
    expect(css).toMatch(/::view-transition-old\(root\) \{ animation: none; opacity: 0; \}/);
  });
});

describe('reduced motion stylesheet', () => {
  const css = readFileSync(`${__dirname}/../src/app/globals.css`, 'utf8');
  const block = css.slice(css.indexOf('@media (prefers-reduced-motion: reduce)'));
  it('collapses animations and transitions and mutes view transitions and shimmer', () => {
    expect(block).toMatch(/animation-duration:\s*0\.01ms !important/);
    expect(block).toMatch(/transition-duration:\s*0\.01ms !important/);
    expect(block).toContain('::view-transition-old(*)');
    expect(block).toContain('.shimmer::after');
  });
});

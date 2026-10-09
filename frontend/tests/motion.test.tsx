import { act, fireEvent, render, screen } from '@testing-library/react';
import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TransitionLink } from '@/components/transition-link';
import { notifyNavigated, runWithTransition } from '@/lib/view-transition';

const push = vi.fn();
vi.mock('next/navigation', () => ({ useRouter: () => ({ push }), usePathname: () => '/' }));

function mockReducedMotion(reduce: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: reduce && q.includes('reduce'), media: q, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia;
}
const doc = document as unknown as { startViewTransition?: unknown };

beforeEach(() => {
  push.mockReset();
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
    vi.stubGlobal('requestAnimationFrame', (cb: () => void) => cb());
    const nav = vi.fn();
    expect(runWithTransition(nav)).toBe(true);
    expect(nav).toHaveBeenCalledTimes(1);
    expect(settled).toBe(false);
    notifyNavigated();
    await Promise.resolve();
    await Promise.resolve();
    expect(settled).toBe(true);
    vi.unstubAllGlobals();
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

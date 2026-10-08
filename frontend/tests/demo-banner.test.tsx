import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DemoBanner } from '@/components/demo-banner';
import { STORAGE_KEY } from '@/lib/demo/store';

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  vi.resetModules();
});

describe('DemoBanner', () => {
  it('says the data stays in the browser and offers a reset', () => {
    render(<DemoBanner />);
    expect(screen.getByRole('note')).toHaveTextContent('Demo — data stays in your browser · Reset');
    expect(screen.getByRole('button', { name: 'Reset' })).toBeInTheDocument();
  });

  it('Reset wipes the saved demo and returns to the dashboard', async () => {
    const assign = vi.fn();
    vi.stubGlobal('location', { ...window.location, assign });
    window.localStorage.setItem(STORAGE_KEY, '{"anything":true}');
    render(<DemoBanner />);
    await userEvent.click(screen.getByRole('button', { name: 'Reset' }));
    expect(assign).toHaveBeenCalled();
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
    expect(String(assign.mock.calls[0]?.[0])).toMatch(/\/dashboard\/$/);
  });
});

describe('postHref', () => {
  it('uses the real path outside the demo', async () => {
    const { postHref } = await import('@/lib/demo/config');
    expect(postHref('abc')).toBe('/posts/abc');
  });

  it('uses the query form in the static demo, which cannot serve unknown path segments', async () => {
    vi.stubEnv('NEXT_PUBLIC_DEMO', 'true');
    vi.resetModules();
    const { postHref, BASE_PATH, SITE_HREF } = await import('@/lib/demo/config');
    expect(postHref('a b')).toBe('/posts/view?id=a%20b');
    expect(BASE_PATH).toBe('/SMSI/demo');
    expect(SITE_HREF).toBe('/SMSI/');
  });
});

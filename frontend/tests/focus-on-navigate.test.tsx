import { render } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const nav = vi.hoisted(() => ({ path: '/login' }));
vi.mock('next/navigation', () => ({ usePathname: () => nav.path }));

import { FocusOnNavigate } from '@/components/focus-on-navigate';

function Page({ withButton }: { withButton: boolean }) {
  return (
    <>
      <main id="main" tabIndex={-1}>
        {withButton ? <button type="button">Sign in</button> : null}
        <a href="/x">Other</a>
      </main>
      <FocusOnNavigate />
    </>
  );
}

const frames = () => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));

beforeEach(() => {
  nav.path = '/login';
});
afterEach(() => {
  document.body.innerHTML = '';
});

describe('FocusOnNavigate', () => {
  it('moves lost focus to <main> after a navigation', async () => {
    const view = render(<Page withButton />);
    (view.getByRole('button') as HTMLButtonElement).focus();
    nav.path = '/dashboard';
    view.rerender(<Page withButton={false} />); // the focused button is gone
    await frames();
    expect(document.activeElement).toBe(document.getElementById('main'));
  });

  it('leaves focus that is still somewhere', async () => {
    const view = render(<Page withButton={false} />);
    const link = view.getByRole('link');
    link.focus();
    nav.path = '/posts';
    view.rerender(<Page withButton={false} />);
    await frames();
    expect(document.activeElement).toBe(link);
  });

  it('does nothing on the first load', async () => {
    render(<Page withButton={false} />);
    await frames();
    expect(document.activeElement).toBe(document.body);
  });
});

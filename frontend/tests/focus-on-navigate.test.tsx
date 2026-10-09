import { act, render } from '@testing-library/react';
import { StrictMode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const nav = vi.hoisted(() => ({ path: '/login' }));
vi.mock('next/navigation', () => ({ usePathname: () => nav.path }));

import { FocusOnNavigate } from '@/components/focus-on-navigate';

// jsdom has no layout: treat an element as rendered unless it (or an ancestor) is `hidden`.
const realRects = Element.prototype.getClientRects;
beforeEach(() => {
  nav.path = '/login';
  Element.prototype.getClientRects = function (this: Element) {
    return (this.closest('[hidden]') ? [] : [{}]) as unknown as DOMRectList;
  };
});
afterEach(() => {
  Element.prototype.getClientRects = realRects;
  vi.useRealTimers();
  document.body.innerHTML = '';
});

function Page({ button = false, main = true, menuHidden = false }: { button?: boolean; main?: boolean; menuHidden?: boolean }) {
  return (
    <>
      <nav hidden={menuHidden}>
        <a href="/posts">Posts</a>
      </nav>
      {main ? (
        <main id="main" tabIndex={-1}>
          {button ? <button type="button">Sign in</button> : null}
        </main>
      ) : null}
      <FocusOnNavigate />
    </>
  );
}

const frames = () => act(() => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))));
const main = () => document.getElementById('main');

describe('FocusOnNavigate', () => {
  it('moves focus to <main> when the focused control was removed', async () => {
    const view = render(<Page button />);
    view.getByRole('button').focus();
    nav.path = '/dashboard';
    view.rerender(<Page />);
    await frames();
    expect(document.activeElement).toBe(main());
  });

  it('moves focus to <main> when the focused link was hidden (mobile menu closing)', async () => {
    const view = render(<Page />);
    view.getByRole('link').focus();
    nav.path = '/posts';
    view.rerender(<Page menuHidden />);
    await frames();
    expect(document.activeElement).toBe(main());
  });

  it('leaves focus that is still on a visible element', async () => {
    const view = render(<Page />);
    const link = view.getByRole('link');
    link.focus();
    nav.path = '/posts';
    view.rerender(<Page />);
    await frames();
    expect(document.activeElement).toBe(link);
  });

  it('does nothing on the first load, also under StrictMode', async () => {
    render(
      <StrictMode>
        <Page />
      </StrictMode>,
    );
    await frames();
    expect(document.activeElement).toBe(document.body);
  });

  it('waits for <main> to mount (the app layout loads first)', async () => {
    const view = render(<Page main={false} />);
    nav.path = '/dashboard';
    view.rerender(<Page main={false} />);
    await frames();
    expect(document.activeElement).toBe(document.body);
    view.rerender(<Page />);
    await frames();
    expect(document.activeElement).toBe(main());
  });

  it('gives up when no <main> appears', async () => {
    const now = vi.spyOn(performance, 'now');
    now.mockReturnValue(0);
    const view = render(<Page main={false} />);
    nav.path = '/dashboard';
    view.rerender(<Page main={false} />);
    now.mockReturnValue(5000);
    await frames();
    view.rerender(<Page />); // too late: the loop has stopped
    await frames();
    expect(document.activeElement).toBe(document.body);
    now.mockRestore();
  });

  it('stops the previous wait on a quick second navigation', async () => {
    const cancel = vi.spyOn(window, 'cancelAnimationFrame');
    const view = render(<Page main={false} />);
    nav.path = '/a';
    view.rerender(<Page main={false} />);
    nav.path = '/b';
    view.rerender(<Page main={false} />);
    expect(cancel).toHaveBeenCalled();
    cancel.mockRestore();
  });
});

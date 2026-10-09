import { act, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { MOTION_KEY, PrefsProvider, usePrefs } from '@/components/prefs-provider';

function Probe() {
  const { motionPaused, setMotionPaused } = usePrefs();
  return (
    <button type="button" onClick={() => setMotionPaused(!motionPaused)}>
      {motionPaused ? 'paused' : 'moving'}
    </button>
  );
}

afterEach(() => {
  window.localStorage.clear();
  document.documentElement.classList.remove('motion-off');
});

describe('Pause motion preference', () => {
  it('toggles the html class and remembers the choice', () => {
    render(<PrefsProvider><Probe /></PrefsProvider>);
    act(() => screen.getByRole('button').click());
    expect(screen.getByRole('button')).toHaveTextContent('paused');
    expect(document.documentElement).toHaveClass('motion-off');
    expect(window.localStorage.getItem(MOTION_KEY)).toBe('off');

    act(() => screen.getByRole('button').click());
    expect(document.documentElement).not.toHaveClass('motion-off');
    expect(window.localStorage.getItem(MOTION_KEY)).toBe('on');
  });

  it('reads a stored choice on load', () => {
    window.localStorage.setItem(MOTION_KEY, 'off');
    render(<PrefsProvider><Probe /></PrefsProvider>);
    expect(screen.getByRole('button')).toHaveTextContent('paused');
  });
});

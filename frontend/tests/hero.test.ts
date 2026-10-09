import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { centerOf, heroFill, heroWipe } from '@/lib/hero';

function mockReducedMotion(reduce: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: reduce && q.includes('reduce'), media: q, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia;
}

// jsdom has no Web Animations; a finished animation is all the helpers need.
const animate = vi.fn(() => ({ finished: Promise.resolve() }));
const proto = HTMLElement.prototype as unknown as { animate?: unknown };

beforeEach(() => {
  mockReducedMotion(false);
  proto.animate = animate;
  animate.mockClear();
});
afterEach(() => {
  delete proto.animate;
  document.querySelectorAll('.hero-overlay').forEach((el) => el.remove());
});

describe('heroFill', () => {
  it('navigates at once, grows an accent circle from the button, then removes the layer', async () => {
    const navigate = vi.fn(() => window.history.pushState({}, '', '/dashboard'));
    heroFill({ x: 10, y: 20 }, navigate);
    expect(navigate).toHaveBeenCalledTimes(1);
    const layer = document.querySelector('.hero-overlay');
    expect(layer).toHaveAttribute('aria-hidden', 'true');
    const [frames] = animate.mock.calls[0] as unknown as [Keyframe[]];
    expect(String(frames[0]?.clipPath)).toBe('circle(0px at 10px 20px)');
    await vi.waitFor(() => expect(document.querySelector('.hero-overlay')).toBeNull());
    expect(animate).toHaveBeenCalledTimes(2);
  });

  it('only navigates under reduced motion', () => {
    mockReducedMotion(true);
    const navigate = vi.fn();
    heroFill({ x: 0, y: 0 }, navigate);
    expect(navigate).toHaveBeenCalledTimes(1);
    expect(document.querySelector('.hero-overlay')).toBeNull();
    expect(animate).not.toHaveBeenCalled();
  });

  it('only navigates where Web Animations are missing', () => {
    delete proto.animate;
    const navigate = vi.fn();
    heroFill({ x: 0, y: 0 }, navigate);
    expect(navigate).toHaveBeenCalledTimes(1);
    expect(document.querySelector('.hero-overlay')).toBeNull();
  });
});

describe('heroWipe', () => {
  it('sweeps once and cleans up', async () => {
    await heroWipe();
    expect(animate).toHaveBeenCalledTimes(1);
    expect(document.querySelector('.hero-overlay')).toBeNull();
  });

  it('cleans up when the animation is cancelled', async () => {
    animate.mockReturnValueOnce({ finished: Promise.reject(new DOMException('cancel', 'AbortError')) });
    await expect(heroWipe()).resolves.toBeUndefined();
    expect(document.querySelector('.hero-overlay')).toBeNull();
  });

  it('does nothing under reduced motion', async () => {
    mockReducedMotion(true);
    await heroWipe();
    expect(animate).not.toHaveBeenCalled();
  });
});

describe('centerOf', () => {
  it('falls back to the middle of the screen', () => {
    expect(centerOf(null)).toEqual({ x: window.innerWidth / 2, y: window.innerHeight / 2 });
  });
});

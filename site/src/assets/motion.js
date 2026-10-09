// Pointer effects for the landing page: magnetic buttons, tilting screens, a light that follows the cursor in the hero.
// Mouse and trackpad only (hover: hover and pointer: fine), never touch, never with reduced motion, never while "Pause motion"
// is on. The page works the same without this file. All writes go through requestAnimationFrame, and only CSS custom
// properties on the element itself change: the CSS turns them into transform, translate and opacity.
const root = document.documentElement;
const fine = matchMedia('(hover: hover) and (pointer: fine)');
const reduced = matchMedia('(prefers-reduced-motion: reduce)');
const allowed = () => fine.matches && !reduced.matches && !root.classList.contains('motion-off');

const clamp = (v, max) => Math.max(-max, Math.min(max, v));
const queue = new Map(); // element -> function that writes its style, flushed once per frame
let scheduled = 0;
const schedule = (el, write) => {
  queue.set(el, write);
  if (!scheduled) {
    scheduled = requestAnimationFrame(() => {
      scheduled = 0;
      const jobs = [...queue.values()];
      queue.clear();
      jobs.forEach((job) => job());
    });
  }
};
const mouse = (e) => e.pointerType === 'mouse';

try {
  // Magnetic: a button leans toward the pointer (CSS: translate, so it composes with the press and entrance transforms).
  for (const el of document.querySelectorAll('[data-magnetic]')) {
    let box = null;
    el.addEventListener('pointerenter', (e) => { if (mouse(e) && allowed()) box = el.getBoundingClientRect(); }, { passive: true });
    el.addEventListener('pointermove', (e) => {
      if (!box || !mouse(e)) return;
      const x = clamp((e.clientX - (box.left + box.width / 2)) * 0.2, 7);
      const y = clamp((e.clientY - (box.top + box.height / 2)) * 0.3, 5);
      schedule(el, () => { el.style.setProperty('--mx', `${x.toFixed(1)}px`); el.style.setProperty('--my', `${y.toFixed(1)}px`); });
    }, { passive: true });
    el.addEventListener('pointerleave', () => {
      box = null;
      schedule(el, () => { el.style.removeProperty('--mx'); el.style.removeProperty('--my'); });
    }, { passive: true });
  }

  // Depth: screens tilt a few degrees toward the pointer and lift a little (CSS reads --rx, --ry, --s).
  for (const el of document.querySelectorAll('[data-tilt]')) {
    const max = el.closest('.stage') ? 3 : 4;
    let box = null;
    el.addEventListener('pointerenter', (e) => {
      if (!mouse(e) || !allowed()) return;
      box = el.getBoundingClientRect();
      schedule(el, () => el.style.setProperty('--s', '1.012'));
    }, { passive: true });
    el.addEventListener('pointermove', (e) => {
      if (!box || !mouse(e)) return;
      const px = (e.clientX - box.left) / box.width - 0.5;
      const py = (e.clientY - box.top) / box.height - 0.5;
      schedule(el, () => { el.style.setProperty('--ry', `${(px * max * 2).toFixed(2)}deg`); el.style.setProperty('--rx', `${(-py * max * 2).toFixed(2)}deg`); });
    }, { passive: true });
    el.addEventListener('pointerleave', () => {
      box = null;
      schedule(el, () => ['--rx', '--ry', '--s'].forEach((p) => el.style.removeProperty(p)));
    }, { passive: true });
  }

  // Light: a soft glow in the hero eases toward the pointer.
  const hero = document.querySelector('[data-hero]')?.closest('.hero');
  const light = hero?.querySelector('[data-light]');
  if (hero && light) {
    let tx = 0, ty = 0, x = 0, y = 0, raf = 0, seeded = false;
    const step = () => {
      raf = 0;
      x += (tx - x) * 0.14;
      y += (ty - y) * 0.14;
      light.style.setProperty('--lx', `${x.toFixed(1)}px`);
      light.style.setProperty('--ly', `${y.toFixed(1)}px`);
      if (Math.abs(tx - x) + Math.abs(ty - y) > 0.5) raf = requestAnimationFrame(step);
    };
    hero.addEventListener('pointermove', (e) => {
      if (!mouse(e) || !allowed()) return;
      const r = hero.getBoundingClientRect();
      tx = e.clientX - r.left;
      ty = e.clientY - r.top;
      if (!seeded) { x = tx; y = ty; seeded = true; }
      light.classList.add('is-on');
      if (!raf) raf = requestAnimationFrame(step);
    }, { passive: true });
    hero.addEventListener('pointerleave', () => light.classList.remove('is-on'), { passive: true });
  }
} catch (err) {
  console.warn('landing: pointer effects are off', err);
}

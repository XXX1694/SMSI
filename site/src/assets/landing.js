// Landing page behaviour. Progressive enhancement: without this file the page is complete, just still.
// Every block is guarded on its own, and the `js` class (which hides .reveal elements until they scroll in) is only set
// once the reveal setup has worked, so one failing feature can never leave the page blank.
const reduced = matchMedia('(prefers-reduced-motion: reduce)');
const dark = matchMedia('(prefers-color-scheme: dark)');
const root = document.documentElement;
const guard = (name, fn) => {
  try {
    return fn();
  } catch (err) {
    console.warn(`landing: ${name} is off`, err);
    return undefined;
  }
};

// 0. Motion switch (WCAG 2.2.2). One toggle stops all motion: the video, the flow canvas, the glow blobs, the marquee,
//    scroll parallax, reveals, the pointer effects and the story route (CSS keys off html.motion-off). The label stays
//    "Pause motion" and the state is exposed with aria-pressed. It is remembered per visitor. With reduced motion the page
//    starts paused and the switch is not offered, because nothing moves.
const KEY = 'socialos_landing_motion';
const readPref = () => guard('motion preference', () => localStorage.getItem(KEY)) ?? null;
const motion = { off: reduced.matches || readPref() === 'off', listeners: [] };
const toggle = document.querySelector('[data-motion]');
const applyMotion = () => {
  root.classList.toggle('motion-off', motion.off);
  if (toggle) toggle.setAttribute('aria-pressed', String(motion.off));
  motion.listeners.forEach((fn) => guard('motion listener', () => fn(motion.off)));
};
if (toggle) {
  if (reduced.matches) toggle.hidden = true;
  toggle.addEventListener('click', () => {
    motion.off = !motion.off;
    guard('save motion preference', () => localStorage.setItem(KEY, motion.off ? 'off' : 'on'));
    applyMotion();
  });
}
root.classList.toggle('motion-off', motion.off);

// 1. Scroll reveals: a class flips once, CSS does the motion (scroll.css). Items that arrive together are staggered.
//    Headlines are first split into words, each in its own mask, so they rise line by line.
guard('split headlines', () => {
  if (reduced.matches) return;
  for (const el of document.querySelectorAll('.split')) {
    // Words are separated by spaces; ja and zh have none, so their catalogs put a zero-width space between phrases.
    const text = el.textContent.trim().replace(/\s+/g, ' ');
    const label = text.replace(/\u200b/g, '');
    el.setAttribute('aria-label', label);
    const parts = text.match(/[^ \u200b]+(?: |\u200b|$)/g) ?? [text];
    el.replaceChildren(
      ...parts.flatMap((part, i) => {
        const word = part.replace(/[ \u200b]$/, '');
        const outer = document.createElement('span');
        outer.className = 'sw';
        outer.setAttribute('aria-hidden', 'true');
        const inner = document.createElement('span');
        inner.className = 'sw-i';
        inner.style.setProperty('--i', String(i));
        inner.textContent = word;
        outer.append(inner);
        const spaced = part.endsWith(' ');
        return spaced ? [outer, ' '] : [outer];
      }),
    );
    el.classList.add('is-split');
  }
});
guard('reveals', () => {
  const reveals = [...document.querySelectorAll('.reveal, .split, .clip')];
  const io = new IntersectionObserver(
    (entries) => {
      entries.filter((e) => e.isIntersecting).forEach((e, i) => {
        if (e.target.classList.contains('reveal') && !e.target.classList.contains('clip')) e.target.style.transitionDelay = `${i * 70}ms`;
        e.target.classList.add('in');
        io.unobserve(e.target);
      });
    },
    { threshold: 0.12, rootMargin: '0px 0px -6% 0px' },
  );
  for (const el of reveals) {
    // Anything already on screen is shown before the hiding class exists, so nothing flashes.
    if (el.getBoundingClientRect().top < innerHeight) el.classList.add('in');
    else io.observe(el);
  }
  root.classList.add('js');
});

// 2. Hero background (its own module, so a failure to load or run it cannot touch anything else).
const canvas = document.querySelector('[data-flow]');
if (canvas) {
  import('./hero-flow.js')
    .then(({ startFlow }) => {
      const flow = startFlow(canvas, { reduced: motion.off });
      motion.listeners.push((off) => flow.setPaused(off));
    })
    .catch((err) => console.warn('landing: flow background is off', err));
}

// 3. Counters: facts only (the numbers come from the build). Start at zero when scrolled into view, end on the true value.
guard('counters', () => {
  if (reduced.matches) return;
  const io = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (!e.isIntersecting) continue;
        io.unobserve(e.target);
        const to = Number(e.target.dataset.count);
        const t0 = performance.now();
        const step = (now) => {
          const k = Math.min((now - t0) / 1100, 1);
          e.target.textContent = String(Math.round(to * (1 - Math.pow(1 - k, 4))));
          if (k < 1) requestAnimationFrame(step);
        };
        e.target.textContent = '0';
        requestAnimationFrame(step);
      }
    },
    { threshold: 0.6 },
  );
  document.querySelectorAll('.num[data-count]').forEach((n) => io.observe(n));
});

// 4. "How it works": the step nearest the middle of the screen drives the sticky picture.
guard('story', () => {
  const story = document.querySelector('[data-story]');
  if (!story) return;
  const stage = story.querySelector('.story-stage');
  const rail = story.querySelector('.steps-list');
  const steps = [...story.querySelectorAll('.step')];
  const io = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (!e.isIntersecting) continue;
        const i = Number(e.target.dataset.step);
        steps.forEach((s, n) => s.classList.toggle('is-active', n === i));
        stage.dataset.active = String(i);
        rail.style.setProperty('--p', String((i + 1) / steps.length));
      }
    },
    { rootMargin: '-45% 0px -45% 0px' },
  );
  steps.forEach((s) => io.observe(s));
});

// 5. Hero video: real footage of the demo. Not on phones, not with reduced motion, not with data saver, not while motion
//    is paused, and only while it is on screen. The poster stays underneath, so nothing jumps when it starts or stops.
guard('hero video', () => {
  const stage = document.querySelector('[data-hero-video]');
  const video = stage?.querySelector('video');
  if (!stage || !video) return;
  const wide = matchMedia('(min-width: 62rem)');
  const saver = navigator.connection?.saveData === true;
  const webm = video.canPlayType('video/webm; codecs="vp9"') !== '';
  let loadedFor = '';
  let onScreen = false;

  const wanted = () => wide.matches && !reduced.matches && !saver && !motion.off;
  const load = () => {
    const scheme = dark.matches ? 'dark' : 'light';
    if (loadedFor === scheme) return;
    loadedFor = scheme;
    stage.classList.remove('is-playing');
    video.src = `${video.dataset.video}${scheme}.${webm ? 'webm' : 'mp4'}`;
    video.load();
  };
  const sync = () => {
    if (wanted() && onScreen) {
      load();
      video.play().catch(() => {});
    } else {
      video.pause();
    }
  };
  video.addEventListener('playing', () => stage.classList.add('is-playing'));
  new IntersectionObserver(([e]) => { onScreen = e.isIntersecting; sync(); }, { threshold: 0.25 }).observe(stage);
  dark.addEventListener('change', sync);
  wide.addEventListener('change', sync);
  motion.listeners.push(sync);
});

applyMotion();

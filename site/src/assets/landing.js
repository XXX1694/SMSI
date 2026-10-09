// Landing page behaviour. Progressive enhancement: without this file the page is complete, just still.
import { startFlow } from './hero-flow.js';

const reduced = matchMedia('(prefers-reduced-motion: reduce)');
const dark = matchMedia('(prefers-color-scheme: dark)');

// 1. Hero background.
const canvas = document.querySelector('[data-flow]');
if (canvas) startFlow(canvas, { reduced: reduced.matches });

// 2. Scroll reveals: a class flips once, CSS does the motion (scroll.css). Items that arrive together are staggered.
const reveals = [...document.querySelectorAll('.reveal')];
if (reveals.length) {
  const io = new IntersectionObserver(
    (entries) => {
      entries.filter((e) => e.isIntersecting).forEach((e, i) => {
        e.target.style.transitionDelay = `${i * 70}ms`;
        e.target.classList.add('in');
        io.unobserve(e.target);
      });
    },
    { threshold: 0.12, rootMargin: '0px 0px -6% 0px' },
  );
  reveals.forEach((el) => io.observe(el));
}

// 3. Counters: facts only (the numbers come from the build). Start at zero when scrolled into view, end on the true value.
if (!reduced.matches) {
  const nums = [...document.querySelectorAll('.num[data-count]')];
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
  nums.forEach((n) => io.observe(n));
}

// 4. "How it works": the step nearest the middle of the screen drives the sticky picture.
const story = document.querySelector('[data-story]');
if (story) {
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
}

// 5. Hero video: real footage of the demo. Not on phones, not with reduced motion, not with data saver, and only while
//    it is on screen. The poster stays underneath, so nothing jumps when the video starts or stops.
const stage = document.querySelector('[data-hero-video]');
const video = stage?.querySelector('video');
const pause = stage?.querySelector('[data-pause]');
if (stage && video) {
  const wide = matchMedia('(min-width: 62rem)');
  const saver = navigator.connection?.saveData === true;
  const webm = video.canPlayType('video/webm; codecs="vp9"') !== '';
  let userPaused = false;
  let loadedFor = '';

  const wanted = () => wide.matches && !reduced.matches && !saver;
  const load = () => {
    const scheme = dark.matches ? 'dark' : 'light';
    if (loadedFor === scheme) return;
    loadedFor = scheme;
    stage.classList.remove('is-playing');
    video.src = `${video.dataset.video}${scheme}.${webm ? 'webm' : 'mp4'}`;
    video.load();
  };
  const play = () => {
    if (!wanted() || userPaused) return;
    load();
    video.play().catch(() => {});
  };
  video.addEventListener('playing', () => { stage.classList.add('is-playing'); if (pause) pause.hidden = false; });
  const io = new IntersectionObserver(([e]) => (e.isIntersecting ? play() : video.pause()), { threshold: 0.25 });
  io.observe(stage);
  dark.addEventListener('change', () => { if (wanted()) play(); });
  const retarget = () => {
    if (wanted()) return;
    video.pause();
    stage.classList.remove('is-playing');
    if (pause) pause.hidden = true;
  };
  reduced.addEventListener('change', retarget);
  wide.addEventListener('change', () => (wanted() ? play() : retarget()));
  pause?.addEventListener('click', () => {
    userPaused = !userPaused;
    pause.setAttribute('aria-pressed', String(userPaused));
    pause.textContent = userPaused ? 'Play video' : 'Pause video';
    userPaused ? video.pause() : play();
  });
}

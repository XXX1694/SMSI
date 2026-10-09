// The hero background: posts leave an agent on the left, stop at an approval gate, then fan out to networks on the right.
// It is decoration (aria-hidden) and it explains the product in one glance. Canvas 2D, paused off screen and when the tab
// is hidden, one static frame under prefers-reduced-motion.
const LANES = 6;
const GATE_T = 0.46;

// CanvasRenderingContext2D.roundRect is missing in older Safari; a plain rectangle is fine at this size.
const pill = (ctx, x, y, w, h, r) => (ctx.roundRect ? (ctx.beginPath(), ctx.roundRect(x, y, w, h, r)) : (ctx.beginPath(), ctx.rect(x, y, w, h)));

export function startFlow(canvas, { reduced }) {
  const ctx = canvas.getContext('2d');
  if (!ctx) return { setPaused() {}, stop() {} };
  const root = getComputedStyle(document.documentElement);
  let w = 0, h = 0, dpr = 1, colors, lanes = [], packets = [], rings = [];
  let raf = 0, last = 0, visible = true, paused = reduced;
  let accentChannel = '';

  // Theme colours are read once per theme change, never inside the frame loop.
  const channel = (name) => root.getPropertyValue(name).trim();
  const hsl = (name, a) => `hsl(${channel(name)} / ${a})`;
  const readColors = () => {
    accentChannel = channel('--accent');
    colors = { line: hsl('--foreground', 0.08), gate: hsl('--foreground', 0.18), accent: hsl('--accent', 0.9), accentSoft: hsl('--accent', 0.3), idle: hsl('--foreground', 0.22), dot: hsl('--foreground', 0.14), bg: hsl('--background', 1) };
  };

  const resize = () => {
    const r = canvas.getBoundingClientRect();
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    w = r.width; h = r.height;
    canvas.width = Math.round(w * dpr); canvas.height = Math.round(h * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const n = w < 700 ? 3 : LANES;
    lanes = Array.from({ length: n }, (_, i) => {
      const k = i / (n - 1);
      const y0 = h * (0.2 + k * 0.62), y1 = h * (0.12 + ((i * 0.37) % 1) * 0.74);
      return { p0: [-30, y0], p1: [w * 0.3, y0 + (k - 0.5) * 60], p2: [w * 0.64, y1 - (k - 0.5) * 90], p3: [w + 30, y1] };
    });
    packets = Array.from({ length: n * 2 }, (_, i) => ({ lane: i % n, t: (i / (n * 2)) + Math.random() * 0.05, speed: 0.045 + Math.random() * 0.03, crossed: false }));
  };

  const at = (l, t) => {
    const u = 1 - t, a = u * u * u, b = 3 * u * u * t, c = 3 * u * t * t, d = t * t * t;
    return [a * l.p0[0] + b * l.p1[0] + c * l.p2[0] + d * l.p3[0], a * l.p0[1] + b * l.p1[1] + c * l.p2[1] + d * l.p3[1]];
  };

  const frame = (dt) => {
    ctx.clearRect(0, 0, w, h);
    ctx.lineWidth = 1;
    for (const l of lanes) {
      ctx.strokeStyle = colors.line;
      ctx.beginPath();
      ctx.moveTo(...l.p0);
      ctx.bezierCurveTo(...l.p1, ...l.p2, ...l.p3);
      ctx.stroke();
      const [gx, gy] = at(l, GATE_T);
      ctx.strokeStyle = colors.gate;
      ctx.beginPath(); ctx.arc(gx, gy, 6, 0, 6.2832); ctx.stroke();
      const [ex, ey] = at(l, 0.985);
      ctx.fillStyle = colors.dot;
      ctx.beginPath(); ctx.arc(ex, ey, 3.5, 0, 6.2832); ctx.fill();
    }
    for (const p of packets) {
      p.t += p.speed * dt;
      // Packets slow down at the gate, the way a request waits for an approval.
      const near = Math.abs(p.t - GATE_T);
      if (!reduced && near < 0.03 && !p.crossed) p.t -= p.speed * dt * 0.55;
      if (!p.crossed && p.t >= GATE_T) { p.crossed = true; const [x, y] = at(lanes[p.lane], GATE_T); rings.push({ x, y, age: 0 }); }
      if (p.t > 1) { p.t = -0.02; p.crossed = false; p.lane = Math.floor(Math.random() * lanes.length); }
      if (p.t < 0) continue;
      const [x, y] = at(lanes[p.lane], p.t);
      ctx.fillStyle = p.crossed ? colors.accent : colors.idle;
      pill(ctx, x - 7, y - 4.5, 14, 9, 2.5); ctx.fill();
      if (p.crossed) { ctx.fillStyle = colors.accentSoft; pill(ctx, x - 20, y - 1.5, 12, 3, 1.5); ctx.fill(); }
    }
    rings = rings.filter((r) => (r.age += dt) < 0.9);
    for (const r of rings) {
      ctx.strokeStyle = `hsl(${accentChannel} / ${0.6 * (1 - r.age / 0.9)})`;
      ctx.beginPath(); ctx.arc(r.x, r.y, 6 + r.age * 26, 0, 6.2832); ctx.stroke();
    }
  };

  const tick = (now) => {
    raf = 0;
    if (!visible || document.hidden || paused) return;
    const dt = Math.min((now - last) / 1000, 0.05);
    last = now;
    frame(dt);
    raf = requestAnimationFrame(tick);
  };
  const run = () => { if (!raf && !paused) { last = performance.now(); raf = requestAnimationFrame(tick); } };

  readColors(); resize();
  const still = () => {
    packets.forEach((p, i) => { p.t = 0.1 + (i / packets.length) * 0.8; p.crossed = p.t > GATE_T; });
    frame(0);
  };
  if (paused) still(); else run();

  const io = new IntersectionObserver(([e]) => { visible = e.isIntersecting; if (visible) run(); });
  io.observe(canvas);
  const ro = new ResizeObserver(() => { resize(); if (paused) still(); });
  ro.observe(canvas);
  const scheme = matchMedia('(prefers-color-scheme: dark)');
  const onScheme = () => { readColors(); if (paused) still(); };
  scheme.addEventListener('change', onScheme);
  document.addEventListener('visibilitychange', run);
  // Pause stops the loop and leaves one still frame, so the picture does not vanish.
  const setPaused = (value) => {
    paused = value;
    if (paused) { cancelAnimationFrame(raf); raf = 0; still(); } else run();
  };
  const stop = () => { cancelAnimationFrame(raf); io.disconnect(); ro.disconnect(); scheme.removeEventListener('change', onScheme); document.removeEventListener('visibilitychange', run); };
  return { setPaused, stop };
}

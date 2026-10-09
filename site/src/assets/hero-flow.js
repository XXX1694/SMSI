// The hero flow: drafts leave an agent on the left, steer toward one approval gate, then fan out to networks on the right.
// It lives behind the product footage (and, on phones, as a slim band above it), never under the text.
// The lanes are the Steerpost mark in motion (a path that bends and lands), and the gate is where the human decides.
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
  let w = 0, h = 0, dpr = 1, colors, lanes = [], packets = [], rings = [], gate = [0, 0];
  let raf = 0, last = 0, visible = true, paused = reduced, compact = false, minDt = 0;
  const narrow = matchMedia('(max-width: 61.99rem)');
  let accentChannel = '';

  // Theme colours are read once per theme change, never inside the frame loop.
  const channel = (name) => root.getPropertyValue(name).trim();
  const hsl = (name, a) => `hsl(${channel(name)} / ${a})`;
  const readColors = () => {
    accentChannel = channel('--accent');
    colors = { line: hsl('--accent', 0.24), gate: hsl('--accent', 0.7), accent: hsl('--accent', 0.9), accentSoft: hsl('--accent', 0.3), idle: hsl('--foreground', 0.22), dot: hsl('--foreground', 0.2), bg: hsl('--background', 1) };
  };

  const resize = () => {
    const r = canvas.getBoundingClientRect();
    compact = narrow.matches;
    // Phones: a lighter canvas (1.5x pixels, 30 frames a second, three lanes) in a slim band.
    dpr = Math.min(window.devicePixelRatio || 1, compact ? 1.5 : 2);
    minDt = compact ? 1 / 31 : 0;
    w = r.width; h = r.height;
    canvas.width = Math.round(w * dpr); canvas.height = Math.round(h * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const n = compact ? 3 : LANES;
    gate = compact ? [w * 0.5, h * 0.5] : [w * 0.14, h * 0.58];
    const pick = (list) => (n === 3 ? [list[0], list[2], list[5]] : list);
    const from = pick([0.12, 0.26, 0.42, 0.6, 0.76, 0.9]), to = pick([0.08, 0.28, 0.5, 0.62, 0.8, 0.94]);
    lanes = from.map((f, i) => ({ y0: h * f, y1: h * to[i] }));
    for (const l of lanes) { l.a = seg(l, true); l.b = seg(l, false); } // computed once per resize, not per frame
    packets = Array.from({ length: n * 2 }, (_, i) => ({ lane: i % n, t: (i / (n * 2)) + Math.random() * 0.05, speed: 0.045 + Math.random() * 0.03, crossed: false }));
  };

  // Each lane is two cubics with flat tangents at the ends: an S that converges on the gate, then one that fans out.
  const X0 = -30;
  const seg = (l, first) => {
    const [gx, gy] = gate;
    const x1 = w + 30;
    return first
      ? [[X0, l.y0], [X0 + (gx - X0) * 0.55, l.y0], [gx - (gx - X0) * 0.45, gy], [gx, gy]]
      : [[gx, gy], [gx + (x1 - gx) * 0.45, gy], [x1 - (x1 - gx) * 0.55, l.y1], [x1, l.y1]];
  };
  const cubic = (p, t) => {
    const u = 1 - t, a = u * u * u, b = 3 * u * u * t, c = 3 * u * t * t, d = t * t * t;
    return [a * p[0][0] + b * p[1][0] + c * p[2][0] + d * p[3][0], a * p[0][1] + b * p[1][1] + c * p[2][1] + d * p[3][1]];
  };
  const at = (l, t) => (t < GATE_T ? cubic(l.a, t / GATE_T) : cubic(l.b, (t - GATE_T) / (1 - GATE_T)));
  const tip = (ctx, l) => {
    const [x, y] = at(l, 0.985), [px, py] = at(l, 0.965);
    const a = Math.atan2(y - py, x - px), s = 5.5;
    ctx.save(); ctx.translate(x, y); ctx.rotate(a);
    ctx.beginPath(); ctx.moveTo(s, 0); ctx.lineTo(-s * 0.7, -s * 0.9); ctx.lineTo(-s * 0.25, 0); ctx.lineTo(-s * 0.7, s * 0.9); ctx.closePath(); ctx.fill();
    ctx.restore();
  };

  const frame = (dt) => {
    ctx.clearRect(0, 0, w, h);
    ctx.lineWidth = 1;
    for (const l of lanes) {
      const { a, b } = l;
      ctx.strokeStyle = colors.line;
      ctx.beginPath();
      ctx.moveTo(...a[0]);
      ctx.bezierCurveTo(...a[1], ...a[2], ...a[3]);
      ctx.bezierCurveTo(...b[1], ...b[2], ...b[3]);
      ctx.stroke();
      ctx.fillStyle = colors.dot;
      tip(ctx, l);
    }
    // One gate for every lane: the place where a person approves.
    ctx.strokeStyle = colors.gate;
    ctx.lineWidth = 1.5;
    ctx.beginPath(); ctx.arc(gate[0], gate[1], 9, 0, 6.2832); ctx.stroke();
    ctx.fillStyle = colors.gate;
    ctx.beginPath(); ctx.arc(gate[0], gate[1], 2.5, 0, 6.2832); ctx.fill();
    ctx.lineWidth = 1;
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
    if (dt < minDt) { raf = requestAnimationFrame(tick); return; }
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
  narrow.addEventListener('change', () => { resize(); if (paused) still(); });
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

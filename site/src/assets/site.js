// Progressive enhancement only: the site is fully readable without this file.

// 1. Copy buttons on code blocks.
for (const pre of document.querySelectorAll('.prose pre:not(.mermaid), .copyable pre')) {
  if (pre.parentElement?.classList.contains('code')) continue;
  pre.tabIndex = 0; // a scrollable block must be reachable by keyboard
  const wrap = document.createElement('div');
  wrap.className = 'code';
  pre.replaceWith(wrap);
  wrap.append(pre);
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'copy';
  btn.textContent = 'Copy';
  btn.setAttribute('aria-label', 'Copy code to clipboard');
  btn.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(pre.innerText.replace(/\n$/, ''));
      btn.textContent = 'Copied';
    } catch {
      btn.textContent = 'Press Ctrl+C';
    }
    setTimeout(() => (btn.textContent = 'Copy'), 1600);
  });
  wrap.append(btn);
}

// Docs menu: open by default (it is a sidebar on wide screens), collapsed on phones.
const docsMenu = document.querySelector('.docs-nav details');
if (docsMenu && window.matchMedia('(max-width: 52rem)').matches) docsMenu.open = false;

// 2. Mermaid diagrams, rendered in the browser (only loaded on pages that have any).
const diagrams = [...document.querySelectorAll('pre.mermaid')];
if (diagrams.length > 0) {
  const sources = diagrams.map((d) => d.textContent ?? '');
  const dark = window.matchMedia('(prefers-color-scheme: dark)');
  let mermaid;
  const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

  const render = async () => {
    if (!mermaid) return;
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      theme: 'base',
      fontFamily: 'Inter Variable, Inter, ui-sans-serif, system-ui, sans-serif',
      themeVariables: {
        darkMode: dark.matches,
        background: 'transparent',
        primaryColor: css('--accent-soft'),
        primaryBorderColor: css('--accent'),
        primaryTextColor: css('--fg'),
        secondaryColor: css('--muted'),
        tertiaryColor: css('--surface'),
        lineColor: css('--muted-fg'),
        textColor: css('--fg'),
        edgeLabelBackground: css('--bg'),
        clusterBkg: css('--surface'),
        clusterBorder: css('--border'),
        attributeBackgroundColorOdd: css('--surface'),
        attributeBackgroundColorEven: css('--muted'),
      },
    });
    diagrams.forEach((d, i) => {
      d.removeAttribute('data-processed');
      d.textContent = sources[i];
    });
    try {
      await mermaid.run({ nodes: diagrams });
    } catch (err) {
      console.warn('Mermaid could not render a diagram; showing its source instead.', err);
    }
    for (const d of diagrams) if (d.querySelector('svg')) makeZoomable(d);
  };

  // Wide diagrams (the database schema) are scaled down to the column; a click shows them at full size.
  const makeZoomable = (d) => {
    if (d.classList.contains('zoomable') || typeof HTMLDialogElement === 'undefined') return;
    d.classList.add('zoomable');
    d.tabIndex = 0;
    d.setAttribute('role', 'button');
    d.setAttribute('aria-label', 'Enlarge diagram');
    const open = () => {
      const svg = d.querySelector('svg');
      if (!svg) return;
      const dlg = document.createElement('dialog');
      dlg.className = 'diagram';
      const close = document.createElement('button');
      close.type = 'button';
      close.className = 'btn btn-secondary btn-sm close';
      close.textContent = 'Close';
      close.addEventListener('click', () => dlg.close());
      const big = svg.cloneNode(true);
      const box = svg.viewBox?.baseVal;
      if (box && box.width) {
        big.setAttribute('width', String(Math.round(box.width)));
        big.setAttribute('height', String(Math.round(box.height)));
      }
      big.style.maxWidth = 'none';
      dlg.append(close, big);
      dlg.addEventListener('click', (e) => e.target === dlg && dlg.close());
      dlg.addEventListener('close', () => dlg.remove());
      document.body.append(dlg);
      dlg.showModal();
    };
    d.addEventListener('click', open);
    d.addEventListener('keydown', (e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), open()));
  };

  try {
    // Served from this site (assets/vendor, copied from the mermaid package by build.mjs): no third-party request.
    await new Promise((resolve, reject) => {
      const tag = document.createElement('script');
      tag.src = new URL('vendor/mermaid.min.js', import.meta.url).href;
      tag.onload = resolve;
      tag.onerror = reject;
      document.head.append(tag);
    });
    mermaid = window.mermaid;
    await render();
    dark.addEventListener('change', render);
  } catch {
    // Script missing or blocked: the diagram source stays visible as plain text.
    for (const d of diagrams) d.setAttribute('title', 'Diagram source (the renderer could not be loaded)');
  }
}

// Page transitions (style.css) only make sense between the landing page and the docs. The demo is a separate app that
// does not opt in, so leaving for it skips the transition instead of letting the browser abort it with an error.
addEventListener('pageswap', (e) => {
  const vt = e.viewTransition;
  if (!vt) return;
  for (const p of [vt.ready, vt.finished, vt.updateCallbackDone]) p.catch(() => {});
  const to = e.activation?.entry?.url;
  if (to && new URL(to).pathname.includes('/demo/')) vt.skipTransition();
});

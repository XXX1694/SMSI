// Language menu and the first-visit suggestion. Progressive enhancement: the menu is a <details> and the footer list is
// plain links, so both work without this file. Nothing here redirects: a visitor is only ever offered a link.
//
// The choice is remembered under the same key the app uses (frontend/src/i18n/resolve.ts), because the site and the
// demo share one origin. The tag matching mirrors frontend/src/i18n/match.ts.
const KEY = 'steerpost_locale';
const guard = (fn, fallback) => {
  try {
    return fn();
  } catch {
    return fallback;
  }
};
const read = () => guard(() => localStorage.getItem(KEY), null);
const write = (v) => guard(() => localStorage.setItem(KEY, v));

const data = guard(() => JSON.parse(document.getElementById('lang-data')?.textContent ?? 'null'), null);

// 1. The menu: closes on Escape, on an outside click and when focus leaves it; picking a language is remembered.
const menu = document.querySelector('[data-lang]');
if (menu) {
  const close = (focus) => {
    if (!menu.open) return;
    menu.open = false;
    if (focus) menu.querySelector('summary')?.focus();
  };
  document.addEventListener('click', (e) => { if (!menu.contains(e.target)) close(false); });
  menu.addEventListener('keydown', (e) => { if (e.key === 'Escape') close(true); });
  menu.addEventListener('focusout', (e) => { if (e.relatedTarget && !menu.contains(e.relatedTarget)) close(false); });
}
for (const a of document.querySelectorAll('[data-locale]')) {
  a.addEventListener('click', () => {
    write(a.dataset.locale);
    // Stay on the same section when the visitor switches language.
    if (location.hash) a.href = `${a.getAttribute('href')}${location.hash}`;
  });
}

// 2. The suggestion: shown once, on the English page, when the browser prefers another listed language.
const mapTag = (tag) => {
  const [lang, ...rest] = String(tag).trim().replace(/_/g, '-').toLowerCase().split('-');
  switch (lang) {
    case 'en': case 'uk': return 'en'; // uk never falls back to ru
    case 'es': return 'es';
    case 'pt': return 'pt-BR';
    case 'fr': return 'fr';
    case 'de': return 'de';
    case 'ru': return 'ru';
    case 'id': case 'in': return 'id';
    case 'ja': return 'ja';
    case 'ar': return 'ar';
    case 'zh': return rest.some((r) => r === 'hant' || r === 'tw' || r === 'hk' || r === 'mo') ? 'en' : 'zh-CN';
    default: return null;
  }
};
const preferred = () => {
  for (const tag of navigator.languages?.length ? navigator.languages : [navigator.language]) {
    const code = mapTag(tag);
    if (code && (code === 'en' || data.locales[code])) return code;
  }
  return 'en';
};

guard(() => {
  if (!data || data.current !== 'en') return;
  const saved = read();
  const want = saved && data.locales[saved] ? saved : preferred();
  const target = data.locales[want];
  if (want === 'en' || !target) return;

  const bar = document.createElement('div');
  bar.className = 'lang-suggest';
  bar.setAttribute('role', 'region');
  bar.setAttribute('aria-label', target.text);
  bar.lang = target.lang;
  bar.dir = want === 'ar' ? 'rtl' : 'ltr';
  const text = document.createElement('p');
  text.textContent = target.text;
  const go = document.createElement('a');
  go.href = target.href + location.hash;
  go.textContent = target.name;
  go.hreflang = target.lang;
  go.addEventListener('click', () => write(want));
  const no = document.createElement('button');
  no.type = 'button';
  no.setAttribute('aria-label', target.dismiss);
  no.innerHTML = '<svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" aria-hidden="true" focusable="false"><path d="M3.5 3.5l9 9M12.5 3.5l-9 9"/></svg>';
  no.addEventListener('click', () => {
    write('en'); // remembered: do not ask again
    bar.remove();
  });
  bar.append(text, go, no);
  document.body.append(bar);
});

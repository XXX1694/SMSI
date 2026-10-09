import { LOCALE_STORAGE_KEY } from '@/i18n/resolve';

// Sets lang/dir from the stored locale before paint, only for locales this build offers. When the stored locale is not
// English (the HTML is always rendered in English), the shell is hidden until the catalog resolves, at most 1.5 s.
export function localeScript(enabled: string[]): string {
  return `try{var A=${JSON.stringify(enabled)},l=localStorage.getItem('${LOCALE_STORAGE_KEY}'),h=document.documentElement;if(l&&A.indexOf(l)>=0){if(h.lang!==l){h.setAttribute('data-i18n-pending','');setTimeout(function(){h.removeAttribute('data-i18n-pending')},1500)}h.lang=l;h.dir=/^ar(-|$)/.test(l)?'rtl':'ltr'}}catch(e){}`;
}


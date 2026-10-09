/**
 * Brand colours as hex, for the places that cannot read CSS variables (web manifest, theme-color meta, raster icons).
 * `tests/tokens.test.ts` derives these from `styles/tokens.css` and fails when they drift, so the HSL tokens stay the source.
 */
export const BRAND_HEX = {
  light: { background: '#ffffff', accent: '#086b81' },
  dark: { background: '#0d1317', accent: '#3ecde0' },
} as const;

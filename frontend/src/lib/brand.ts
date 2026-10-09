/**
 * Brand colours as hex, for the places that cannot read CSS variables (web manifest, theme-color meta, raster icons). The
 * browser chrome takes `canvas`, the page colour behind the glass (D-024).
 * `tests/tokens.test.ts` derives these from `styles/tokens.css` and fails when they drift, so the HSL tokens stay the source.
 */
export const BRAND_HEX = {
  light: { background: '#ffffff', canvas: '#edf3f5', accent: '#086b81' },
  dark: { background: '#0d1317', canvas: '#0a1015', accent: '#3ecde0' },
} as const;

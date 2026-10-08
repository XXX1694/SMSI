/** Tiny flat SVG thumbnails for demo media (no image files to ship). */
export function svgThumb(label: string, hue: number): string {
  const text = label.replace(/[<>&"']/g, '').slice(0, 28);
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="400" viewBox="0 0 400 400">` +
    `<rect width="400" height="400" fill="hsl(${hue},38%,88%)"/>` +
    `<circle cx="300" cy="120" r="64" fill="hsl(${hue},45%,76%)"/>` +
    `<rect x="48" y="236" width="304" height="10" rx="5" fill="hsl(${hue},40%,70%)"/>` +
    `<rect x="48" y="262" width="210" height="10" rx="5" fill="hsl(${hue},40%,78%)"/>` +
    `<text x="48" y="204" font-family="sans-serif" font-size="26" font-weight="600" fill="hsl(${hue},40%,24%)">${text}</text>` +
    `</svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}

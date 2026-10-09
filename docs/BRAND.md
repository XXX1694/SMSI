# Steerpost brand

Steerpost means **the human steers, the agent posts**. The brand has to say that quietly: a calm developer product that is
precise about what it does and honest about what it does not. This file is the reference for the name, the voice, the logo, the
colours, the type, the motion and the imagery. Tokens live in `frontend/src/styles/tokens.css`; generated files come from
`site/scripts/build-brand.mjs`. Do not copy values from here into components: use the tokens.

## 1. Name

- Write **Steerpost**: one word, capital S, lower-case rest. Never `STEERPOST`, `SteerPost`, `Steer Post` or `steerpost` in
  running text.
- Slug and technical identifiers: `steerpost` (repository, image names, package scope, domain labels).
- Suggested short name for the CLI and the MCP server entry: `steer` (`steer login`, `"steer": { ... }` in a client config).
  The full name stays in docs, UI and titles.
- The tagline is **The human steers. The agent posts.** Two sentences, two full stops. The descriptor is "self-hosted social
  publishing for people and their AI agents".
- Possessive and plural are fine in prose ("Steerpost's approval gate"). Do not turn the name into a verb.
- The rename of existing copy from SocialOS is a separate change. This one touches only the logo, the wordmark, the favicon,
  the landing hero brand and the app sidebar brand.

## 2. Voice

Calm, precise, honest. "You stay in control" is the promise, so the writing never hurries the reader.

| Do | Don't |
|---|---|
| Say what happens and what it needs: "Waits here for your yes." | Hype: "supercharge", "seamless", "next-gen". |
| Name limits and review requirements plainly. | Hide a limitation behind a friendly sentence. |
| Short sentences, concrete verbs, one term per concept. | Metaphors that need explaining (no ship puns in UI copy). |
| Address the person: "you approve", "your agent drafts". | Personify the product ("Steerpost thinks...") or the agent as a person. |
| Errors: what happened, how to fix it. | "Oops", exclamation marks, apologies without a next step. |

The steering image is a design motif, not a copy theme. "Steer" appears in the tagline and the CLI name; the interface does
not talk about helms, courses or sailing.

## 3. Logo

### Concept

The mark is a path that **leaves flat, bends up in two equal quarter turns and lands in a paper-plane tip**. It reads as three
things at once: a lane change (steering), an S (the name), and a post on its way (sending). The two bends are the same radius
and the stroke is constant, so it stays calm; the tip is the only sharp thing, because that is where the action happens. In
motion the path eases in and the tip settles after it (section 7).

### Files

All in `frontend/public/brand/` (served by the app at `/brand/...`) and `site/src/assets/brand/`:

| File | Use |
|---|---|
| `mark.svg`, `wordmark.svg`, `lockup.svg` | `currentColor`: inherit text colour; for inline `<img>` use the light/dark variants. |
| `mark-light.svg`, `mark-dark.svg` | Fixed brand colour for light and dark backgrounds. |
| `wordmark-light.svg`, `wordmark-dark.svg` | Name only, in the text colour. |
| `lockup-light.svg`, `lockup-dark.svg` | Mark in the brand colour plus name: the default logo. |
| `app-icon.svg`, `icon-192.png`, `icon-512.png`, `icon-maskable-512.png` | App icon (white mark on the brand colour), PWA manifest. |
| `social-preview.png` | 1280 x 640 link preview (also `docs/assets/social-preview.png`). |

Also: `site/src/assets/favicon.svg`, `favicon-32.png`, `apple-touch-icon.png` (180 px), the same icons in
`frontend/src/app/`, and `docs/assets/banner-light.svg` / `banner-dark.svg` for the README. The wordmark is Inter outlines
(see section 6), so it renders the same without the font. Regenerate everything with
`cd site && CHROMIUM_PATH=... node scripts/build-brand.mjs`; colours come from the tokens.

### Rules

- **Clear space**: keep one tip-height (about a quarter of the mark's height) free on every side of the mark or lockup.
- **Minimum size**: mark 16 px (favicon uses the tile), lockup 20 px tall (about 100 px wide). Below that, use the mark alone.
- **Colour**: brand colour on neutral backgrounds, or a single flat colour (foreground, white, black). White on the brand
  colour for the app icon. Dark theme uses the lighter brand colour, never the light-theme one.
- **Position**: mark left of the name, centred on the x-height line. The mark may be used alone when the name is nearby.
- **Don't**: rotate, mirror or outline the mark; stretch or recolour the wordmark; set the name in another font; add shadows,
  glows or gradients; put the mark on a busy photo; put it on a background it has less than 3:1 contrast against; redraw the
  curve with sharper bends or a thicker tip; use the tile shape for the logo inside the product (the tile is for icons only).

## 4. Colour

One accent, deep **harbour teal**. It is not purple or blue, so it does not look like every other AI product, and it is dark
enough in light mode to carry white text (6.1:1). Neutrals are cool slate with a hint of the same hue. Roles are fixed:

- **accent**: primary actions, links, focus ring, the logo, the active nav bar, the "sent" lane in the hero. One per view.
- **foreground / muted-foreground**: text. **background / surface / muted**: the page, raised areas, quiet fills.
- **border / input**: dividers and control outlines. **accent-soft**: tinted fill behind accent text.
- **success / warning / danger**: status only, always paired with a word or an icon, never colour alone.

Hex is derived from the HSL tokens (`frontend/src/lib/brand.ts` mirrors the four values that need hex, and a test keeps them
in sync). Contrast is computed by `frontend/tests/tokens.test.ts` for every text pair; AA (4.5:1) is a hard floor.

### Light

| Token | Hex | HSL |
|---|---|---|
| background | `#ffffff` | `0 0% 100%` |
| surface | `#f7f9fa` | `196 20% 97.5%` |
| foreground | `#131e25` | `205 32% 11%` |
| muted | `#f0f3f4` | `196 16% 95%` |
| muted-foreground | `#515e67` | `205 12% 36%` |
| border | `#dfe4e7` | `200 14% 89%` |
| input | `#788891` | `202 10% 52%` |
| **accent / ring** | `#086b81` | `191 88% 27%` |
| accent-foreground | `#ffffff` | `0 0% 100%` |
| accent-soft | `#e7f5f8` | `190 55% 94%` |
| success / soft | `#1c6938` / `#e9f7ee` | `142 58% 26%` / `142 45% 94%` |
| warning / soft | `#915108` / `#fdf3dd` | `32 90% 30%` / `40 90% 93%` |
| danger / soft | `#b42222` / `#fdeded` | `0 68% 42%` / `0 80% 96%` |

### Dark

| Token | Hex | HSL |
|---|---|---|
| background | `#0d1317` | `205 28% 7%` |
| surface | `#12191e` | `205 24% 9.5%` |
| foreground | `#eef1f2` | `195 14% 94%` |
| muted | `#1b2228` | `205 20% 13%` |
| muted-foreground | `#a0abb1` | `200 10% 66%` |
| border | `#242d32` | `205 16% 17%` |
| input | `#5e6d78` | `205 12% 42%` |
| **accent / ring** | `#3ecde0` | `187 72% 56%` |
| accent-foreground | `#0e151b` | `205 30% 8%` |
| accent-soft | `#142e34` | `190 45% 14%` |
| success / soft | `#51c882` / `#132a1d` | `145 52% 55%` / `145 38% 12%` |
| warning / soft | `#f5b547` / `#2e230f` | `38 90% 62%` / `38 50% 12%` |
| danger / soft | `#f47171` / `#321515` | `0 85% 70%` / `0 40% 14%` |

### Verified text pairs (WCAG contrast ratio)

| Pair | Light | Dark |
|---|---|---|
| foreground on background | 17.0 | 16.5 |
| muted-foreground on background / muted | 6.7 / 6.0 | 8.0 / 6.9 |
| accent-foreground on accent | 6.1 | 9.6 |
| accent on background / accent-soft | 6.1 / 5.5 | 9.8 / 7.4 |
| success on background / soft | 6.7 / 6.1 | 8.9 / 7.2 |
| warning on background / soft | 6.2 / 5.6 | 10.4 / 8.6 |
| danger on background / soft | 6.6 / 5.8 | 6.7 / 5.9 |

Control outlines (`input`) are held at 3:1 or better against both `background` and `surface` (WCAG 1.4.11); a test enforces it.

## 5. Iconography and imagery

- **UI icons**: keep lucide (already in the app), 16 px in the sidebar and 20 px elsewhere, 1.5 to 2 px stroke, `currentColor`,
  `aria-hidden` unless the icon is the only label. Network marks are Simple Icons (CC0), single colour.
- **Illustration**: none. The brand's picture is the steering curve: smooth S-shaped lanes that converge on one gate (a ring
  with a dot) and fan out again, one lane in the accent colour ending in the plane tip. Hairline strokes (1.5 px), no fills,
  low contrast except the accent lane. It appears in the hero, the banner and the social preview.
- **Screens, not stock**: product shots are real captures of the demo (`site/scripts/screenshots.mjs`, `record-hero.mjs`).
  No stock photos, no people, no 3D renders, no gradients as decoration beyond the soft accent glow in the hero.

## 6. Typography

Keep the vendored **Inter** (variable, OFL, `site/src/assets/fonts/Inter-OFL.txt`; `@fontsource-variable/inter` in the app).
Reasons: it is already shipped and licensed, one variable file covers every weight we use, it has tabular figures for counts
and times, and it stays legible at the 11 to 13 px sizes a dense dashboard needs. A second family would add a file, a licence
to audit and a flash of unstyled text for little gain; the identity comes from the mark, the colour and the motion, not from a
display face.

- Wordmark: Inter at weight 620, tracking -0.022em, converted to outlines. UI copy uses live text at 600 for the name.
- Headings: weight 600 to 660, tracking -0.04em to -0.05em at display sizes; body at 400 and 1.5 line height.
- Numbers in tables and counters: `font-variant-numeric: tabular-nums`.
- Code: the system monospace stack. Do not load a webfont for it.

## 7. Motion

Motion explains; it never decorates the controls. Everything honours `prefers-reduced-motion` (no loops, nothing hidden until
revealed), animates `transform` and `opacity` only, and the landing keeps its Pause control (D-017).

| Token | Value | Use |
|---|---|---|
| `--duration-fast` / `-base` / `-slow` | 150 / 200 / 320 ms | hover, menus, enters |
| `--duration-path` | 900 ms | a drawn line: the logo path, the hero lanes |
| `--ease-standard` | `cubic-bezier(0.4, 0, 0.2, 1)` | moves within the screen |
| `--ease-enter` / `--ease-exit` | `(0.16, 1, 0.3, 1)` / `(0.4, 0, 1, 1)` | things arriving / leaving (exit is faster) |
| `--ease-steer` | `cubic-bezier(0.45, 0, 0.15, 1)` | the steering curve: slow start, committed middle, soft landing |

**The steering motif.** A line eases in, bends, and settles on its target. Used in two places only: the logo (the path draws
and the tip follows, once, on first paint) and the hero (lanes of drafts bend toward one gate, wait there, then fan out). Packets slow at the gate because a request waits for a person. Do not reuse the
motif for hover states, toggles or page transitions.

## 8. Where each asset is used

App sidebar and mobile header: `components/brand/logo.tsx`. Browser tab: `app/icon.svg`, `app/icon.png`, `app/apple-icon.png`,
`app/manifest.ts`, theme-color in `app/layout.tsx`. Landing: nav and footer word, hero tag, `hero-flow.js` (colours from
tokens at runtime), `og:image` = `assets/brand/social-preview.png`. README: `docs/assets/wordmark-*.svg` and the banners.

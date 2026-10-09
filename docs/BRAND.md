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
enough in light mode to carry white text (6.1:1). Neutrals are cool slate with a hint of the same hue. A second, quieter
teal (**lagoon**) structures the screen so the accent can stay rare (D-024). Roles are fixed:

- **accent**: primary actions, links, focus ring, the logo, the active nav bar, chart series 1, the "sent" lane in the hero,
  the fill of the few hero transitions (section 7). One primary action per view.
- **secondary** (lagoon): the tint of the sidebar and header glass, the active nav row, selected and hovered rows, secondary
  buttons, eyebrow labels, chart series 2 and 3, the hero lanes and the landing mesh. `secondary-foreground` is the text on it.
- **foreground / muted-foreground**: text. **canvas / background / surface / muted**: the page behind the glass, solid
  reading surfaces (tables, inputs, long text), raised areas, quiet fills.
- **border / input**: dividers and control outlines. **accent-soft**: tinted fill behind accent text.
- **success / warning / danger / info**: status only, always paired with a word and a glyph shape, never colour alone.
  `info` (blue) is for neutral notices and "in progress"; it is never used where the accent would do.

### Proportions

Colour is spent by area. **App: 70 / 20 / 10** (dominant neutrals / lagoon / accent): a dense dashboard needs the neutrals to
carry the screen, and in practice the accent lands nearer 5 %. **Landing: 60 / 30 / 10**, the classic split, because the mesh
and the tinted glass bands count as secondary and the brand should read above the fold. If a screen looks teal, something
that should be neutral took the accent.

### Light

| Token | Hex | HSL |
|---|---|---|
| canvas | `#edf3f5` | `197 26% 94.5%` |
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
| secondary / foreground | `#d4ecef` / `#1d5560` | `187 45% 88.5%` / `190 54% 24.5%` |
| secondary-strong | `#3a8e9c` | `189 46% 42%` |
| success / soft | `#1c6938` / `#e9f7ee` | `142 58% 26%` / `142 45% 94%` |
| warning / soft | `#915108` / `#fdf3dd` | `32 90% 30%` / `40 90% 93%` |
| danger / soft | `#b42222` / `#fdeded` | `0 68% 42%` / `0 80% 96%` |
| info / soft | `#1f5abf` / `#e8effc` | `218 72% 43.5%` / `219 77% 95%` |
| chart-1 … 4 | `#086b81` `#3a8e9c` `#8fb9c1` `#94a3ab` | accent, secondary-strong, `190 29% 66%`, `201 12% 62.5%` |

### Dark

| Token | Hex | HSL |
|---|---|---|
| canvas | `#0a1015` | `207 35% 6%` |
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
| secondary / foreground | `#14333b` / `#9fd8e0` | `192 50% 15.5%` / `187 51% 75%` |
| secondary-strong | `#5cb4c1` | `188 45% 56%` |
| success / soft | `#51c882` / `#132a1d` | `145 52% 55%` / `145 38% 12%` |
| warning / soft | `#f5b547` / `#2e230f` | `38 90% 62%` / `38 50% 12%` |
| danger / soft | `#f47171` / `#321515` | `0 85% 70%` / `0 40% 14%` |
| info / soft | `#85adfc` / `#16233e` | `220 95% 75.5%` / `220 47% 16.5%` |
| chart-1 … 4 | `#3ecde0` `#5cb4c1` `#2f707c` `#5e6d78` | accent, secondary-strong, `189 45% 33.5%`, input |

### Verified text pairs (WCAG contrast ratio)

Hex is derived from the HSL tokens (`frontend/src/lib/brand.ts` mirrors the values that need hex, and a test keeps them in
sync). Contrast is computed by `frontend/tests/tokens.test.ts` for every text pair; AA (4.5:1) is a hard floor. Text on glass is
checked against the **worst case**: the glass tint composited over the strongest point of the mesh.

| Pair | Light | Dark |
|---|---|---|
| foreground on background | 17.0 | 16.5 |
| muted-foreground on background / muted | 6.7 / 6.0 | 8.0 / 6.9 |
| accent-foreground on accent | 6.1 | 9.6 |
| accent on background / accent-soft | 6.1 / 5.5 | 9.8 / 7.4 |
| secondary-foreground on secondary | 6.8 | 8.4 |
| success on background / soft | 6.7 / 6.1 | 8.9 / 7.2 |
| warning on background / soft | 6.2 / 5.6 | 10.4 / 8.6 |
| danger on background / soft | 6.6 / 5.8 | 6.7 / 5.9 |
| info on background / soft | 6.4 / 5.5 | 8.4 / 7.1 |
| foreground on chrome / card / strong glass (worst case) | 14.3 / 15.9 / 16.5 | 14.2 / 14.4 / 15.0 |
| muted-foreground on chrome / card glass (worst case) | 5.6 / 6.3 | 6.9 / 7.0 |
| accent on chrome / card glass (worst case) | 5.2 / 5.8 | 8.4 / 8.6 |

Control outlines (`input`) are held at 3:1 or better against `background`, `surface` and `canvas` (WCAG 1.4.11); a test
enforces it. Inputs never sit on glass.

### Glass and the mesh

The product's surfaces are frosted glass over a soft teal mesh (D-024). The glass is a material for **chrome and floating
layers**, not for reading.

| Layer | Token | Light | Dark | Blur |
|---|---|---|---|---|
| Sidebar, app header, landing nav | `--glass-chrome` | `hsl(188 43% 93% / 0.70)` | `hsl(201 36% 9% / 0.66)` | 20 px, saturate 160 % |
| Cards, stat tiles, panels | `--glass-card` | `hsl(0 0% 100% / 0.78)` | `hsl(204 29% 10% / 0.74)` | none |
| Popovers, menus, toasts | `--glass-strong` | `hsl(0 0% 100% / 0.90)` | `hsl(204 29% 10% / 0.90)` | 24 px, saturate 160 % |
| Landing hero frame | `--glass-card` | as card | as card | 16 px |

- Edge: a 1 px `--glass-border` (`hsl(205 32% 11% / 0.08)` / `hsl(0 0% 100% / 0.08)`) plus a 1 px inner top highlight
  (`--glass-highlight`). Elevation: `--shadow-glass` (a teal-tinted soft shadow in light, a deep one in dark).
- **The mesh** is one fixed layer behind the page: three large radial gradients (teal `--mesh-1`, mint `--mesh-2`, sky
  `--mesh-3`, 18 to 30 % alpha) on `canvas`, plus a 3 to 5 % grain. It never scrolls, so glass over it stays cheap; cards use
  no backdrop blur because the mesh is already soft.
- **Never glass**: tables, inputs, the editor and composer, long text (Terms, Privacy, docs), dense lists. These are solid
  `background`. Text never sits on glass with less than 0.66 alpha.
- **Budget**: at most two blurred layers on screen (chrome plus one popover or dialog). No blur on anything that scrolls.
  Dialogs are a near-solid panel over an overlay; the overlay alone carries a light blur.
- **Fallbacks**: without `backdrop-filter` the chrome is `secondary` mixed into `background` and floating layers are solid;
  `prefers-reduced-transparency` makes every glass layer solid and the mesh flat; `forced-colors` drops fills, shadows and
  the mesh and draws `CanvasText` borders.

## 5. Iconography, imagery and status tags

- **UI icons**: keep lucide (already in the app), 16 px in the sidebar and 20 px elsewhere, 1.5 to 2 px stroke, `currentColor`,
  `aria-hidden` unless the icon is the only label. Network marks are Simple Icons (CC0), single colour.
- **Illustration**: none. The brand's picture is the steering curve: smooth S-shaped lanes that converge on one gate (a ring
  with a dot) and fan out again, one lane in the accent colour ending in the plane tip. Hairline strokes (1.5 px), no fills,
  low contrast except the accent lane. It appears in the hero, the banner and the social preview.
- **Screens, not stock**: product shots are real captures of the demo (`site/scripts/screenshots.mjs`, `record-hero.mjs`).
  No stock photos, no people, no 3D renders. The only decorative gradients are the mesh behind the glass (section 4) and the
  soft accent glow in the hero.
- **Status tags** are tags, not candy pills: 22 px tall, 5 px radius, a 1 px `border` hairline, no pastel fill, the label in
  the text colour at 12 px / 520 with tabular figures. Only a 12 px **glyph** carries the status colour, and each status has
  its own glyph shape, so colour is never the only cue: draft = dashed ring, awaiting approval = half-filled ring,
  scheduled = clock, publishing = open arc, published = filled check, failed = triangle with a bang (and a tinted border),
  cancelled or expired = slashed ring. Capability and metadata tags are the same shape without a glyph; an unsupported one
  is dashed and struck through, with a screen-reader word. Counts (nav, tabs) are a small neutral square with tabular
  figures, not a coloured dot.

## 6. Typography

**Onest** (variable, weights 100 to 900, OFL) for Latin and Cyrillic, with a matched Noto family per script that only its
locale loads (D-024). Onest was chosen over Inter, Geologica, Golos Text, Rubik, IBM Plex Sans, Manrope and Unbounded: it
covers every Kazakh letter (Әә Ғғ Ққ Ңң Өө Ұұ Үү Һһ Іі, checked in the font's cmap; Manrope and Unbounded miss them), it has
tabular figures, it reads well at 11 to 13 px, and its open, slightly warm forms (the single-storey `y`, the round `a`) give
the product a voice of its own without a display face. Its Latin subset is 32 KB, lighter than Inter's 47 KB.

| Script | Locales | Family | Loaded |
|---|---|---|---|
| Latin, Cyrillic | en, es, pt-BR, de, fr, id, ru, kk | Onest Variable | always; each subset only when a page uses it (`unicode-range`) |
| Arabic | ar | Noto Sans Arabic Variable | only when `<html lang="ar">` |
| Japanese | ja | Noto Sans JP Variable | only when `<html lang="ja">`, and only the slices the page needs |
| Simplified Chinese | zh-CN | Noto Sans SC Variable | only when `<html lang="zh-CN">`, and only the slices the page needs |

- All fonts are **self-hosted**: `@fontsource-variable/*` packages bundled by the app, copied by `site/build.mjs` for the
  landing. No request ever goes to a font CDN. Onest comes first in every stack, so Latin words inside Arabic, Japanese or
  Chinese text stay in the brand face. The language switcher's endonyms on other pages use system fonts, so listing
  `日本語` never downloads a Japanese font.
- Wordmark: unchanged for now, Inter at weight 620 converted to outlines (a drawn logo, not live text). Redrawing it in
  Onest is a separate change. UI copy uses live text at 600 for the name.
- Headings: weight 600 to 660, tracking -0.035em to -0.045em at display sizes (no negative tracking in Arabic, Japanese and
  Chinese); body at 400 and 1.5 line height (1.75 for Japanese and Chinese, 1.8 for Arabic).
- Numbers in tables, tags and counters: `font-variant-numeric: tabular-nums`.
- Code: the system monospace stack. Do not load a webfont for it.

## 7. Motion

Motion explains; it never decorates the controls. Everything honours `prefers-reduced-motion` and the **Pause motion**
setting (the landing since D-017, the app from D-024): no loops, nothing hidden until revealed, only `transform`, `opacity`
and `clip-path` animate.

| Token | Value | Use |
|---|---|---|
| `--duration-fast` / `-base` / `-slow` | 150 / 200 / 320 ms | hover, menus, enters |
| `--duration-hero` | 520 ms | the three hero transitions below, nothing else |
| `--duration-path` | 900 ms | a drawn line: the logo path, the hero lanes |
| `--ease-standard` | `cubic-bezier(0.4, 0, 0.2, 1)` | moves within the screen |
| `--ease-enter` / `--ease-exit` | `(0.16, 1, 0.3, 1)` / `(0.4, 0, 1, 1)` | things arriving / leaving (exit is faster) |
| `--ease-fill` | `cubic-bezier(0.65, 0, 0.35, 1)` | a colour fill or wipe crossing the screen |
| `--ease-steer` | `cubic-bezier(0.45, 0, 0.15, 1)` | the steering curve: slow start, committed middle, soft landing |

**How often decides how much.** Frequent navigation (sidebar, tabs, filters, back, anything from the keyboard) is instant or a
150 ms fade at most. A first entry to a section may rise in once (320 ms). Lists do not stagger, except the dashboard's first
load in a session.

**Hero transitions** are kept for three rare moments that change what the person is doing, each at most `--duration-hero`
and never blocking input:

1. Signing in, and finishing onboarding: an accent circle fills from the button that was pressed, then the new screen is
   revealed behind it.
2. Publishing now: a diagonal accent wipe crosses the screen; the success state lands after it.
3. Scheduling from the composer: the post card moves into its slot in the calendar (a shared-element transition).

They use the View Transitions API, fall back to a plain overlay animation where it is missing, and become a 120 ms
cross-fade under reduced motion or Pause motion.

**The steering motif.** A line eases in, bends, and settles on its target. Used in two places only: the logo (the path draws
and the tip follows, once, on first paint) and the hero (lanes of drafts bend toward one gate, wait there, then fan out).
Packets slow at the gate because a request waits for a person. Do not reuse the motif for hover states, toggles or page
transitions.

## 8. Where each asset is used

App sidebar and mobile header: `components/brand/logo.tsx`. Browser tab: `app/icon.svg`, `app/icon.png`, `app/apple-icon.png`,
`app/manifest.ts`, theme-color in `app/layout.tsx`. Landing: nav and footer word, hero tag, `hero-flow.js` (colours from
tokens at runtime), `og:image` = `assets/brand/social-preview.png`. README: `docs/assets/wordmark-*.svg` and the banners.

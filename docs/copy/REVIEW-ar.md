# Review: landing page, العربية (ar)

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.ar.json`, first drafted with the PR that added it.

Register: Modern Standard Arabic; Latin digits; layout is right-to-left.

To mark the review done, change the status to `native-reviewed`, add the reviewer and date, and drop the beta note from the
page (`footer.beta` in the catalog and `isEn` in `site/build.mjs`).

## Strings that need a native look

- RTL: check the mirrored hero, the sticky story column, the footer language list and the arrow icons on a real device.
- `hero.lead` and `rel.2.text` use all six plural categories (zero, one, two, few, many, other).
- Latin names inside Arabic sentences (LinkedIn, Telegram, MCP) rely on the browser's bidi: check punctuation next to them (`mcp.setup`, `mcp.ask`).
- `facts.trackers` «أدوات تتبّع»; `mcp.risk.*` «حساس / حرج».
- Glossary ar column was empty: terms are proposals.
- Fonts: system Arabic fonts (Geeza Pro, Segoe UI, Noto); the display weight on macOS is Geeza Pro Bold, check it reads well on Windows and Android.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

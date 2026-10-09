# Review: landing page, Español (es)

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.es.json`, first drafted with the PR that added it.

Register: Neutral international Spanish, tú.

To mark the review done, change the status to `native-reviewed`, add the reviewer and date, and drop the beta note from the
page (`footer.beta` in the catalog and `isEn` in `site/build.mjs`).

## Strings that need a native look

- `hero.title` wording «Los agentes de IA redactan»; `mcp.risk.sensitive` «sensible» (glossary: Sensible).
- `look.b.cap` «Panel» for Dashboard (glossary).
- `rel.2.text` plural «veces».
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

# Review: landing page, Bahasa Indonesia (id)

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.id.json`, first drafted with the PR that added it.

Register: Anda; established loans (akun, unggah, pos).

To mark the review done, change the status to `native-reviewed`, add the reviewer and date, and drop the beta note from the
page (`footer.beta` in the catalog and `isEn` in `site/build.mjs`).

## Strings that need a native look

- `hero.l1` «Agen AI menyusun pos»; `mcp.f1` «cakupan» for scope.
- `facts.retries` «Percobaan ulang, lalu galat yang jelas»: «galat» vs «error» (developers often write error).
- Glossary id column was empty: terms are proposals.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

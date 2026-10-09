# Review: landing page, Português (Brasil) (pt-BR)

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.pt-BR.json`, first drafted with the PR that added it.

Register: Brazilian forms only, você.

To mark the review done, change the status to `native-reviewed`, add the reviewer and date, and drop the beta note from the
page (`footer.beta` in the catalog and `isEn` in `site/build.mjs`).

## Strings that need a native look

- `hero.l1` «escrevem posts» (post is the accepted term).
- `net.live` «Ativa» is feminine to agree with «rede»; check against the other badges.
- `footer.text` «auto-hospedada».
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

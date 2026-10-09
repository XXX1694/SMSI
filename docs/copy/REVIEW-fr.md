# Review: landing page, Français (fr)

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.fr.json`, first drafted with the PR that added it.

Register: vous; narrow no-break spaces (U+202F) before : ; ? !.

To mark the review done, change the status to `native-reviewed`, add the reviewer and date, and drop the beta note from the
page (`footer.beta` in the catalog and `isEn` in `site/build.mjs`).

## Strings that need a native look

- Check every U+202F placement, including inside « … » in `mcp.ask`.
- `hero.l1` «Les agents IA rédigent les posts» (post kept; alternative: publication).
- `facts.trackers` «Traqueurs»; `look.b.cap` «Tableau de bord».
- Glossary fr column was empty: terms here (approbation, portée, jeton, auto-hébergé) are proposals.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

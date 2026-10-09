# Review: landing page, 日本語 (ja)

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.ja.json`, first drafted with the PR that added it.

Register: です/ます, no あなた, full-width punctuation, no space between Japanese and Latin (JTF).

To mark the review done, change the status to `native-reviewed`, add the reviewer and date, and drop the beta note from the
page (`footer.beta` in the catalog and `isEn` in `site/build.mjs`).

## Strings that need a native look

- `hero.l2` 「主導権は常に手元に。」: pronoun-free rendering of "You stay in control".
- `route.gate` 「人が承認」 (avoids あなた).
- `nav.networks` 「対応SNS」 (glossary: SNS).
- Phrase breaks: `how.title`, `mcp.title`, `net.title` use U+200B between phrases for the headline reveal.
- Glossary ja terms follow the existing column.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

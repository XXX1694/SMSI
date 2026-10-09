# Review: 日本語 (ja)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.ja.json`, first drafted with the PR that added it.

Register: です/ます, no あなた, full-width punctuation, no space between Japanese and Latin (JTF).

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- `hero.l2` 「主導権は常に手元に。」: pronoun-free rendering of "You stay in control".
- `route.gate` 「人が承認」 (avoids あなた).
- `nav.networks` 「対応SNS」 (glossary: SNS).
- Phrase breaks: `how.title`, `mcp.title`, `net.title` use U+200B between phrases for the headline reveal.
- Glossary ja terms follow the existing column.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

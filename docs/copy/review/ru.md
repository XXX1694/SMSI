# Review: Русский (ru)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.ru.json`, first drafted with the PR that added it.

Register: The owner reviews ru natively.

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- `hero.lead`: plural for the network count («в 5 соцсетей»); check «со своего сервера» as the rendering of self-hosted.
- `how.2.*`, `mcp.f2`: «Approvals» and «Developer» stay English because the screenshots and the app are English; decide whether to quote them.
- `mcp.setup`: deliberately does not name Claude (languages.md: no "works with Claude" on ru pages).
- `rel.2.text`: plural «до 5 раз».
- Button vs body register: «Попробовать демо» (infinitive) against «Создайте…» (polite imperative).
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

# Review: Deutsch (de)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.de.json`, first drafted with the PR that added it.

Register: du, lower case.

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- `nav.how` «So funktioniert’s»; `hero.l1` «KI-Agenten entwerfen Beiträge».
- `mcp.f1` keeps «Scope» (glossary de: Scope); `mcp.f2` «Freigaben» for approvals.
- Length: `cta.demo`, `nav.demo` («Demo testen») were checked at 320 px.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

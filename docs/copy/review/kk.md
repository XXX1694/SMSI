# Review: Қазақша (kk)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer). Hidden: built at `/kk/` with `noindex`, not linked, not in the switcher or the sitemap.
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.kk.json`, first drafted with the PR that added it.

Register: сіз; hidden and noindex until a native reviewer signs this file.

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- Every string. The glossary rows marked "confirm" (нобай for draft, кері қайтарып алу for revoke, болдырмау for cancel) are used as written.
- `rel.2.text` «ретке дейін» with a number: check suffix harmony.
- `hero.l1` «ЖИ агенттері» (ЖИ = жасанды интеллект).
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

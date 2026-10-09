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

## App

Status: `machine-draft`. Hidden: `kk` is not enabled in `frontend/src/i18n/locales.ts` and stays out of the switcher until a native reviewer signs this file.
Catalog: `frontend/messages/kk/*.json` (687 keys, all of `messages/en/*.json`).

Register: «сіз», buttons as verbal nouns (-у), Cyrillic. Steerpost never takes a case suffix: sentences are restructured («Steerpost жүйесіне кіру»); network names are followed by a separate word («{network} желісінде»), not a suffix.

### Terminology chosen

- Dashboard «Шолу», Compose «Жазу», Posts «Жазбалар», Approvals «Мақұлдаулар», Developer «Әзірлеушілер» (new), Media «Медиа».
- «API кілті», «MCP қосылымы», «рұқсат аясы» (scope), «рұқсат», «сенімді кілт», «нобай», «аккаунт», «әлеуметтік желі / желі».
- Glossary rows marked "confirm" are used as written: нобай (draft), кері қайтарып алу (revoke), болдырмау (cancel post), `Болдырылмаған` (canceled status).
- AI agents are «ЖИ агенттері», as on the landing.
- «Тазалау» is used for Reset (demo, custom text), a shorter word than «қалпына келтіру».
- Plurals: kk has one/other and the noun stays singular after a number, so both branches match; the unit endings «# сұранысқа дейін», «# тіркемеге» are written inside the branches.

### Strings that need a native look

- Every string. Highest risk: suffix harmony after Latin words and numbers (`composer.mediaLimits` «10 МБ дейін», `settings.usage.agentsLimit`, `composer.v.overLimit`, `composer.v.mediaMax`), `accounts.telegram.step1` («Хабарламаларды жариялау» should be Telegram's own name of the right), `approvals.minLeft`, `calendar.moreOnDay`.
- Phrases built around placeholders: `posts.scheduledFor`, `posts.createdAt`, `accounts.meta`, `developer.mcp.lastSeen` use «…: {when}» to avoid endings.
- `settings.usage.line` «{limit} ішінен {used}».
- Machine-drafted from `en.json` with `glossary.csv`; no back-translation sample done yet. Meaning-critical: `errors.*`, `*.revokeBody`, `developer.trusted.*`, `composer.publishConfirmBody`.

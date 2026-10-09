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

## App

Status: `machine-draft`. Enabling `ru` in `frontend/src/i18n/locales.ts` is a separate change; it then shows "Beta translation".
Catalog: `frontend/messages/ru/*.json` (687 keys, all of `en.json`). The owner reviews this locale natively.

Register: «вы», buttons in the infinitive, body text in the polite imperative, ё written, «…» quotes.

### Terminology chosen (glossary rows, with the app-only decisions marked new)

- Dashboard «Обзор», Compose «Написать», Media (nav, library) «Медиатека», Approvals «Одобрения», Developer «Разработчикам» (nav limit is 16 characters, «Для разработчиков» does not fit; new).
- «API-ключ», «MCP-подключение», «область доступа» (scope, Developer screens only), «разрешение» elsewhere, «доверенный ключ», «черновик», «пост», «аккаунт», «соцсеть».
- Failed is «Ошибка» for statuses; for the dashboard stat and section it reads «С ошибкой» (new). Unconfirmed «Не подтверждено»; an attempt that runs is «Начата».
- Risk scale in Developer: «Безопасно / Средний / Опасно» (the landing says «рискованно / критично»; the app scale has three levels, new).
- The language option reads «(бета-перевод)», as the landing footer does.
- `common.haveSaved` is «Значение сохранено» to avoid the gendered «Я сохранил(а)».
- The AI-agent onboarding hint does not name Claude (languages.md: no "works with Claude" for ru). The MCP client tabs and the example name `Claude Desktop` stay, they are product names in a config.

### Strings that need a native look

- Plurals with a number and a noun after «не более» / «из»: `settings.usage.agentsLimit`, `composer.v.mediaMax`, `dashboard.onboarding.progress`, `composer.counter` (genitive).
- `accounts.telegram.step1`: «Публикация сообщений» should match Telegram's own name of the admin right.
- `posts.editBlockedOther`, `composer.conflictLocked`: rephrased as full sentences per status because the English fragment does not decline.
- `approvals.status.*`, `approvals.requestedBy` («Запрос: {agent}», the agent name is not declined), `approvals.minLeft`.
- `composer.title` «Написать» as a page title; `composer.publishTo` «Опубликовать в» before account chips.
- `composer.discardTitle` «Отбросить несохранённые изменения?»; `developer.audit.colResource` «Объект».
- `accounts.caps.schedule` «Планирует сама сеть» (scheduling done by the network itself).
- Length: `common.retry` «Повторить публикацию», `common.tryAgain`, `composer.saveDraft` and a few buttons are above 140 % of English because the glossary fixes them; they wrap.
- Machine-drafted from `en.json` with `glossary.csv`; no back-translation sample done yet. Errors and confirmations (`errors.*`, `posts.cancelBody`, `*.revokeBody`, `developer.trusted.*`, `composer.publishConfirmBody`) are the meaning-critical part.

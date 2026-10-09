# Review: Español (es)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.es.json`, first drafted with the PR that added it.

Register: Neutral international Spanish, tú.

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- `hero.title` wording «Los agentes de IA redactan»; `mcp.risk.sensitive` «sensible» (glossary: Sensible).
- `look.b.cap` «Panel» for Dashboard (glossary).
- `rel.2.text` plural «veces».
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

## App

Status: `machine-draft` (shown as "Beta translation").
Reviewer: none yet. Date: none yet. Catalog: `frontend/messages/es.json` (676 keys), first drafted with the PR that added it.

Register: Neutral international Spanish (es-419 vocabulary), tú. Buttons use the infinitive («Guardar», «Programar»), body text
the tú imperative («Conecta una cuenta.»). No exclamations. Quotes “…”.

### Terminology

- post = «publicación» (feminine); statuses agree: Programada, Publicada, Publicada en parte, Fallida, Cancelada (glossary).
  Attempt statuses agree with «intento» (masculine): Iniciado, Correcto, Fallido.
- network = «red social» in sentences; «red» only in short labels (`composer.allNetworks`). account = «cuenta».
- approval = «aprobación», request = «solicitud»; `Waiting` = «En espera» (glossary); the tab `approvals.tabWaiting` is
  «Esperan tu decisión».
- test network is not used in the app catalog; dangerous action = «acción peligrosa»; risk scale Seguro / Medio / Peligroso.
- scope = «ámbito» (glossary) for API-key scopes; permission = «permiso» for MCP connections and error text.
- capability = «función» (glossary). API key = «clave de API». trusted key = «clave de confianza».
- Dashboard = «Panel», Settings = «Configuración», Developer = «Desarrollo» (glossary silent; chosen for the nav limit).
- Plurals: every plural has `one`, `many`, `other`. `many` (exact millions) uses «# de publicaciones»; `other` is used for all
  other counts. Where English has a unit abbreviation («min») `many` is the same as `other`.
- Videos and "email" follow es-419: «video», «correo electrónico».

### Strings that need a native look

- `nav.developer`, `developer.title`: «Desarrollo» (more natural may be «Desarrolladores», but it is 15 characters for a
  13-character budget).
- `posts.writePost`, `dashboard.onboarding.steps.post.action`: «Redactar» alone, to stay within the button length budget.
- `composer.contentScope`: «Ámbito del contenido» (screen-reader name of the tabs; "scope" reused from the glossary).
- `approvals.tabWaiting`: «Esperan tu decisión» for "Waiting for you".
- `posts.createdAt`, `posts.targetMeta`, `accounts.meta`, `developer.mcp.lastSeen`: a colon before `{when}` («Creada:
  {when}») so relative and absolute dates both read well.
- `accounts.tokenConnect.required`, `httpsUrl`, `tooLong`, `invalid`: «El campo “{field}” …» to avoid gender agreement with
  an unknown field label.
- `developer.scopes.*.description`: infinitive («Listar…», «Publicar…») for imperative English descriptions.
- `settings.usage.summary` and `settings.timeZoneHint`: «Periodo», «zona horaria» wording and punctuation.
- `accounts.telegram.step1`: «Publicar mensajes» as Telegram’s Spanish name of the admin right; check against the client.
- `dashboard.onboarding.progress`: «{done} de # pasos hechos» (agreement in `one`).
- `shell.demo.banner` and `posts.noTargets` («Sin cuentas» for "No targets").
- Length: `nav.posts`, `nav.settings`, `common.tryAgain`, `common.newPost`, `auth.signIn`, `posts.cancelPost`,
  `accounts.reconnect`, `composer.saveDraft`, `developer.audit.actorApiKey` exceed 140 % of English but stay within the
  `maxLength` in `meta.json` (glossary terms win); check them at 320 px.
- All strings are machine-drafted from `en.json` with `glossary.csv`; none has been back-translated yet.

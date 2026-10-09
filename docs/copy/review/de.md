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

## App

Status: `machine-draft` (shown as "Beta translation").
Reviewer: none yet. Date: none yet. Catalog: `frontend/messages/de/*.json` (676 keys), first drafted with the PR that added it.

Register: du, lower case; buttons in the infinitive («Speichern», «Planen»), body text in the du-imperative («Verbinde ein Konto.»);
quotes „…“. Gendered person nouns are avoided by addressing "du" ("Admin" instead of "Administrator", "Wer" instead of "Akteur").

### Terminology

- Beitrag (post), Netzwerk (network), Konto (account; "target" is also Konto), Entwurf, planen/Geplant: from the glossary.
- Freigabe / Freigaben / Freigeben / Ablehnen for approvals; «gefährliche Aktion» and «vertrauenswürdiger Schlüssel» as in the glossary.
- Testnetzwerk is not used in the app catalog (no key needs it); Scope stays «Scope» on Developer screens, «Berechtigung» elsewhere.
- «API-Schlüssel» with the API in Latin; Webhook, Token, Developer, Audit-Log, MCP-Verbindung as in the landing.
- Not in the glossary, chosen here: Tarif (plan), Kennzahl (metric), Mediathek (library), Voreinstellungen (preferences),
  Nutzungsbedingungen / Datenschutzerklärung (Terms / Privacy Policy; footer link «Datenschutz»), Zeitzone, Einmalcode.

### Strings that need a native look

- `common.status.post.publishing` «Wird veröffentlicht» (19 chars, 140 % budget is 14): glossary term, check badge width at 320 px.
- `common.retry` «Erneut senden» (13) vs `common.tryAgain` «Erneut versuchen»: two close verbs; glossary requires both.
- `approvals.status.consumed` «Freigegeben und ausgeführt», `approvals.status.approved` «Freigegeben, wartet auf den Agenten»: long badges.
- `common.status.account.expired` «Neu verbinden nötig» and `accounts.reconnect` «Neu verbinden» (glossary has «erneut verbinden»; shortened for length).
- `composer.saveChanges` «Speichern» and `developer.apiKeys.createKey` «Erstellen»: shortened to fit the button budget; the object is only implied.
- `composer.conflictLocked`, `posts.editBlockedOther`: the sentence moved inside the `select` ("Er …" refers to the post).
- `accounts.telegram.step1` «Nachrichten posten»: check Telegram's own German label for the admin right.
- `accounts.demoToken`: «ein geheimer Wert, der „invalid“ enthält»; "invalid" stays English because the user types it.
- `accounts.disconnectBody`: «Für das Konto {account} geplante Beiträge …» avoids a case ending on the placeholder; check the flow.
- `developer.trusted.warning`: tone of «in falsche Hände gerät … sich falsch verhält» against the English "leaks / misbehaves".
- `common.haveSaved` «Ist gespeichert» (button, 15-char budget): a native may prefer «Gespeichert».
- `settings.theme` «Design» (appearance); `developer.audit.colActor` «Wer»; `calendar.more` «+# weitere»: short and possibly cryptic.
- `legal.terms` «Nutzungsbedingungen» (19 chars in the footer) at 320 px.
- Length exceptions kept for glossary terms: `common.status.post.failed` / `target.failed` «Fehlgeschlagen», `composer.publishNow`,
  `posts.cancelPost`, `nav.settings` (13, max 16). Everything else is within 140 % or the key's `maxLength`.

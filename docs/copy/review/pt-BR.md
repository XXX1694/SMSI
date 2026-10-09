# Review: Português (Brasil) (pt-BR)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.pt-BR.json`, first drafted with the PR that added it.

Register: Brazilian forms only, você.

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- `hero.l1` «escrevem posts» (post is the accepted term).
- `net.live` «Ativa» is feminine to agree with «rede»; check against the other badges.
- `footer.text` «auto-hospedada».
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

## App

Status: `machine-draft` (shown as "Beta translation").
Reviewer: none yet. Date: none yet. Catalog: `frontend/messages/pt-BR/*.json` (676 keys, same structure as `messages/en/*.json`).

Register: você. Buttons use the infinitive (Salvar, Agendar), body text uses the imperative (Conecte uma conta.). Brazilian
forms only (tela, arquivo, usuário). Quotes “…”.

### Terminology choices

- post (masculine, so statuses read Agendado, Publicado, Cancelado), rascunho, agendar, publicar agora (glossary).
- network = rede social in prose, shortened to rede; account = conta. Account statuses agree with the feminine noun (Ativa,
  Revogada, Expirada for keys). Approval requests are masculine (Expirado, Negado).
- approval = aprovação, request = pedido, Deny = Negar, "Approved, waiting for the agent" per glossary.
- scope = escopo (Developer screens only); elsewhere permissão. Test network, trusted key = chave confiável.
- dangerous action = ação perigosa; Retry = Repetir envio, Try again = Tentar de novo (kept different, as in the glossary).
- Plurals: every plural has `one`, `many` and `other`. `many` (exact millions only) uses the "de" form: "# de posts".
  `one` also covers 0 and 1.5 in pt, so the wording stays neutral where possible.
- Where the glossary was silent: Compose in the page title is "Escrever"; capability = recurso (badge group), audit
  "Resource" = objeto, "Actor" = autor, Unconfirmed attempt = Não confirmado, Succeeded = Com sucesso.

### Strings that need a native look

- `common.retry` «Repetir envio» is 13 characters against a 10-character budget; kept because the glossary fixes it.
- `common.tryAgain`, `composer.saveDraft`, `nav.settings`: one or two characters over the 140% budget (glossary terms).
- `accounts.telegram.step1` «Postar mensagens» as Telegram's own admin-right name; verify against Telegram pt-BR.
- `accounts.telegram.connectedTo` «Agora você pode publicar nesse chat»: gender-neutral wording for an unknown chat type.
- `auth.welcomeBack` «Bom ver você de novo.» avoids the gendered "Bem-vindo".
- `common.status.attempt.succeeded` «Com sucesso» and `...attempt.unknown` «Não confirmado» (masculine for a feminine
  noun, to match the glossary).
- `developer.apiKeys.badgeAsks` «Pergunta antes de ações perigosas» is long for a badge; check at 320 px.
- `developer.audit.colResource` «Objeto» and `accounts.caps.label` «Recursos» (both English words map to "recurso").
- `approvals.action.post_retry_now` «Repetir envio agora» and `post_schedule_soon` «Agendar para os próximos minutos».
- `approvals.requestedBy` «Pedido de {agent}» (the glossary says "pedido de aprovação"; agent name is not inflected).
- `composer.saveAnyway` «Salvar mesmo assim» drops "mine"; the dialog context should make the meaning clear.
- `developer.mcp.bridgeIntro` «Ponte» for "Bridge" and «fixado em uma versão exata».
- `language.pseudo` «Pseudolocale (teste)» (developer-only option).
- `calendar.more` «+# mais» is identical in all plural forms by design.

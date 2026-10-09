# Review: Français (fr)

One section per surface. The app's catalog state is `REVIEW` in `frontend/src/i18n/locales.ts`; the landing's is `review` in
`site/i18n/locales.mjs`.

## Landing

Status: `machine-draft` (shown as "Beta translation" in the page footer).
Reviewer: none yet. Date: none yet. Catalog: `site/i18n/landing.fr.json`, first drafted with the PR that added it.

Register: vous; narrow no-break spaces (U+202F) before : ; ? !.

To mark the review done, set `review: 'native-reviewed'` for this locale in `site/i18n/locales.mjs` (the page then drops the
"Beta translation" note), and record the reviewer, date and catalog commit above.

### Strings that need a native look

- Check every U+202F placement, including inside « … » in `mcp.ask`.
- `hero.l1` «Les agents IA rédigent les posts» (post kept; alternative: publication).
- `facts.trackers` «Traqueurs»; `look.b.cap` «Tableau de bord».
- Glossary fr column was empty: terms here (approbation, portée, jeton, auto-hébergé) are proposals.
- All strings are machine-drafted from `landing.en.json` with `glossary.csv`; none has been back-translated yet. Errors and
  confirmations do not appear on the landing page, so the risk is wording and tone, not meaning.
- Docs and the demo stay English; links to them carry "(EN)".

## App

Status: `machine-draft` (shown as "Beta translation").
Reviewer: none yet. Date: none yet. Catalog: `frontend/messages/fr/*.json` (676 keys, same structure and order as `en.json`).

Register: vous. Buttons use the infinitive («Enregistrer», «Planifier»), body text the imperative («Connectez un compte.»). No
exclamation marks. Sentence case. Full sentences end with a period; labels, buttons and badges do not.

### Terminology

Taken from the glossary: réseau, compte, brouillon, post (masculine; statuses «Publié», «Échoué», «Annulé»), planifier,
publier, publier maintenant, approbation, clé d’API, clé de confiance, réseau de test, jeton, portée (Developer screens only),
autorisation (elsewhere), déconnecter, révoquer, outil, agent, connexion MCP, journal d’audit, Tableau de bord, Calendrier.

Glossary conflict: the task brief mentions «Programmer/Programmé», the glossary says «planifier». The glossary wins, so
"schedule" is «planifier», status «Planifié» (same as the landing page).

Gaps in the fr column, filled here (term -> choice):

- upload -> «importer» (media «Importer des médias»); «Téléverser» is 10 characters and Quebec-leaning
- media library -> «médiathèque»; analytics -> «statistiques» (nav and tab), metric -> «indicateur»
- time zone -> «fuseau horaire»; settings -> «Paramètres»; plan -> «offre»; usage -> «utilisation»
- sign in -> «Connexion» (button and title), sign out -> «Déconnexion»; create account -> «Créer un compte»
- email -> «e-mail»; password reset -> «réinitialisation du mot de passe»; verify -> «vérifier»
- text / main text / custom text -> «texte» / «texte principal» / «texte personnalisé»
- publishing (status) -> «En publication»; failed -> «Échoué» (badge) and «En échec» (counters, section titles, sentences)
- unconfirmed -> «Non confirmé» (target), «Non confirmée» (attempt, feminine)
- needs reconnecting -> «Reconnexion requise»; expired key -> «Expirée»
- retry -> «Relancer» (republish), try again -> «Réessayer» (reload); retry now -> «Relancer maintenant»
- attempt -> «tentative»; request -> «demande»; deny -> «Refuser»; approve -> «Approuver»
- approval request statuses are feminine («Approuvée», «Refusée», «Expirée») because they describe a «demande»
- dangerous action -> «action dangereuse»; trusted -> «de confiance»; one-time code -> «code à usage unique»
- capability -> «fonctionnalité»; attach -> «joindre»; preview -> «aperçu»
- cancel post -> «Annuler le post», dismiss «Le garder» (kept short for the length budget)
- reset (demo, custom text) -> «Rétablir» («Réinitialiser» is too long for those buttons)

### Punctuation spaces

Narrow no-break space (U+202F) before `:` `;` `?` `!` and inside « … », written as real characters in the JSON (the
landing file escapes them as ` `). A colon before a value, for example «Créé : {when}», gets U+202F too. Apostrophes are
always ’ (U+2019) because a straight `'` is an ICU quoting character. Numbers and units come from placeholders; file-size
units use the French octet forms («o», «Ko», «Mo»).

### Plural `many`

Every plural has `one`, `many` and `other`. `one` covers 0, 1 and 1.5; `many` only fires for exact millions and takes the "de"
form: «# de posts», «# d’images», «# d’autres», «# de caractères». Phrases without a counted noun repeat the same text in all
three categories («Il reste # min»).

### Strings that need a native look

- `common.status.post.publishing` / `common.status.target.publishing` «En publication»: a noun-like badge for "Publishing".
- `common.status.account.expired` «Reconnexion requise» (19 characters) and `common.status.target.needs_review` «Non confirmé».
- `composer.saveDraft` «Enregistrer»: lost the object "draft" to fit the 14-character budget; same word as `composer.saveChanges`.
- `posts.keepPost` «Le garder» and `posts.cancelPost` «Annuler le post»: check that the pair reads clearly in the dialog.
- `shell.demo.reset`, `shell.demo.resetConfirm`, `composer.reset` «Rétablir»: "restore" instead of "reset"; confirm it is
  understood as "erase my changes".
- `settings.usage.summary` «Offre : <b>{plan}</b>…»: the plan name comes from the server in English ("free").
- `accounts.telegram.step1` «Publier des messages»: Telegram's French name for the admin right; verify against the client.
- `accounts.telegram.connectBody` and `yourChat`: «discussion» for Telegram "chat"; sentence was reshaped from the English.
- `accounts.tokenConnect.required` / `invalid` / `tooLong`: reworded as «Le champ « {field} » …» to avoid gender agreement.
- `dashboard.stats.failed` and `dashboard.sections.failed.title` «En échec» versus the badge «Échoué».
- `approvals.tabWaiting` «À traiter» for "Waiting for you".
- `developer.apiKeys.badgeTrusted` «De confiance : agit sans demander» and `developer.trusted.warning`: tone of the warning.
- `developer.mcp.connectorIntro` and `bridgeIntro`: technical wording («passerelle» for "Bridge", «paquet» for "package").
- `settings.legal` «Mentions légales» for a section that holds the Terms and the Privacy Policy.
- Length over the 140% rule, accepted: `nav.dashboard` (glossary term, within maxLength 16), `shell.openMenu` (accessible name),
  `settings.changePassword`, `composer.publishNow` (glossary term), `calendar.today`, `accounts.telegram.copyCode`,
  `developer.apiKeys.copyKey` (1 to 2 characters over).

# How translations are produced and checked

Inputs: [languages.md](languages.md) (which locales), [style-guide.md](style-guide.md) (how to write),
[glossary.csv](glossary.csv) (which words). This page covers the pipeline. The engineering team implements it in PR-sized
steps; the schema and API changes below need a plan under AGENTS.md section 2.

## 1. Source of truth

- **The English catalog is the only source.** It holds ICU MessageFormat strings. Every other locale is derived from it.
  Nobody edits a translation without a matching English key.
- Layout:
  - `frontend/messages/{locale}.json` for the dashboard and demo, nested by area (`composer.publishNow`).
  - `frontend/messages/meta.json` holds per-key metadata for translators and checks:
    `{ "composer.publishNow": { "type": "button", "description": "Publishes immediately; opens a confirmation", "maxLength": 18 } }`.
    `type` is one of `button | tab | badge | nav | title | body | error | toast | aria`.
  - `site/i18n/landing.{locale}.json` for the landing page (rendered by `site/build.mjs`; the locale registry is
    `site/i18n/locales.mjs`). Its review notes live in `docs/copy/review/{locale}.md`.
  - `backend/internal/adapters/mail/messages/{locale}.json` for email subjects and bodies.
- Keys name a purpose, not the English text: `posts.detail.cancelPost.confirmButton`, not `"Cancel post"`. Never reuse a key
  in two contexts. "Cancel" (dismiss) and "Cancel post" are different keys, even when a language translates them the same.
- **A change of meaning gets a new key**, so stale translations cannot survive. A typo fix keeps the key.
- Complete sentences with named arguments: `{count, plural, one {# attempt} other {# attempts}}`, `{network}`, rich tags such
  as `<link>…</link>`. No concatenation and no `s` suffixes (style guide §3).
- What stays English is listed in languages.md: MCP tools and hints, API `message`, code identifiers.

## 2. Producing a translation

1. A PR that adds or changes English keys does not have to add other locales. A locale is enabled in
   `frontend/src/i18n/locales.ts` only when its catalog has every English key and the checks below pass.
2. **Draft**: an LLM gets the key, the English text, the `meta` entry, the glossary row for every term the string contains,
   the style-guide section for that language and a screenshot of the screen. It returns ICU text only.
3. **Review**: a native reviewer per locale records the result in `docs/copy/review/{locale}.md` (status
   `machine-draft` or `native-reviewed`, reviewer, date, catalog commit, open questions). The owner reviews `ru`. `kk`
   needs a recruited native reviewer, every `kk` glossary row marked "confirm" is settled first, and `kk` stays out of
   the language switcher until it is reviewed.
4. **Beta translation rule (D-021).** A release is blocked only by missing keys or failed checks, never by review status.
   A locale whose review status is `machine-draft` ships labelled "Beta translation" in the language switcher (the honesty
   rule in AGENTS.md section 7). Pull requests do not wait for reviewers either.
5. Glossary changes go through a PR to `glossary.csv` with a reason. A changed term triggers a re-check of every string that
   uses it.

## 3. Automated checks (`npm run i18n:check`, part of `make lint`)

They run offline, in a few seconds, with no new services (AGENTS.md section 6). Each failure names the file, the key and
the rule.

| # | Check | Severity |
|---|---|---|
| 1 | **ICU syntax**: every message parses with `@formatjs/icu-messageformat-parser`. | error |
| 2 | **Placeholders preserved**: the target has exactly the English argument names, with the same type (`plural`, `select`, `number`, `date`), and the same rich tags with the same nesting. | error |
| 3 | **Plural categories complete**: every `plural` contains all categories of `new Intl.PluralRules(locale).resolvedOptions().pluralCategories` (ru and uk: one/few/many/other; es and pt: one/many/other; ar: all six), plus `other`. Exact cases (`=0`) are allowed as extras. The same applies to `selectordinal`. | error |
| 4 | **No missing or extra keys**: the target key set equals the English one. | error |
| 5 | **No untranslated keys**: a target identical to English fails, unless it consists only of do-not-translate terms and placeholders ("MCP", "{network}") or the key is marked `untranslatedOk` in `meta`. | error |
| 6 | **No English left** in non-English catalogs: strip placeholders, tags, code spans, URLs, the do-not-translate list (style guide §14) and glossary rows whose translation equals the English term (pt-BR "post"). Then flag remaining Latin-script words. Cyrillic and CJK locales flag any of them. es, pt-BR and de flag words found in a list of the 5,000 most common English words. | error |
| 7 | **Length budget**: `button`, `tab`, `badge` and `nav` keys are at most **140 %** of the English length (at least 10 characters allowed for short English strings) and within `maxLength` when set. `title` keys are at most 160 %. Length means display width: East Asian wide and full-width characters count as 2. | error for button, tab, badge, nav; warning otherwise |
| 8 | **Glossary terms**: when the English text contains a glossary term, the target contains the approved translation. The match is by stem, because ru, uk and kk inflect. A do-not-translate term in English must appear verbatim. | warning for stems, error for do-not-translate terms |
| 9 | **Typography per locale**: no `!` in UI strings; "…" not "..."; quotes «» for ru, uk and kk, „“ for de, 「」 for ja; uk apostrophe ’; ja full-width 。、; no double, leading or trailing spaces; ru and uk never use a capitalized «Вы»/«Ви» mid-sentence. | error |
| 10 | **Render smoke**: format every message in every locale with sample arguments and with each plural category's CLDR sample number (ru: 1, 2, 5, 1.5). Nothing may throw, and no `{` or `}` may be left in the output. | error |
| 11 | **English metadata**: every English key has a `description`, and every key shown on a control has a `type`. | error |

Also:

- **Types.** The `Messages` type is generated from `en.json`, so using a missing key fails `tsc`.
- **No literals.** `npm run i18n:literals` (part of `npm run lint`, and `tests/i18n-literals.test.ts`) parses every file in
  `src/components`, `src/app` and `src/lib` and fails on JSX text, on `aria-label`, `title`, `placeholder`, `alt` and the copy
  props of our components (`label`, `description`, `hint`, `note`, `confirmLabel`, `dismissLabel`, `retryLabel`), and on
  string literals that read like UI sentences. `react/jsx-no-literals` was not usable: it cannot see attributes without
  also flagging every `className`. The allow-list is `frontend/scripts/i18n-literals.allow.json`: each entry has a reason.
  Strings that stay English by decision: the Terms and Privacy text, static `metadata`, the web manifest, the demo seed and
  engine, and API error messages. `tests/i18n-extract.test.tsx` renders screens in `en-XA` and fails on any plain English
  that is left on screen.
- **How code asks for text.** Components call `useTranslations('ns')` (or the root `useTranslations()`). Server components
  that cannot call hooks use `<T k="ns.key" />`. Functions in `src/lib` stay pure: they take the root translator as a
  parameter (`postStatusView(status, t)`, `validateComposer(state, accounts, providers, t)`) and tests pass `enT` from
  `@/i18n/en`. Errors: `useErrorText()` / `errorMessage(e, t)` map the API `code` to a catalog sentence and show the server
  message only in English.
- **Pseudo-locales in dev.** `en-XA` (accented, 40 % longer, wrapped in [ ]) and `ar-XB` (RTL pseudo) expose hardcoded
  strings, clipping and RTL bugs before any translator starts.

## 4. Review

- **Back-translation sample.** A model or person who did **not** produce the translation translates a sample back to
  English without seeing the source:
  - the sample is 10 % of new or changed keys, at least 20;
  - it always includes every new `error`, `button` and dangerous-confirmation key, and every key with a `plural`.
  - The reviewer compares the meaning with the English source and records each difference in the PR as one of: meaning
    changed, negation lost, consequence lost (what happens and what stays), wrong address form (formal or informal), or a
    glossary term missing.
  - **Any meaning change in an error, a confirmation or a capability text blocks the merge.** More than 5 % minor
    differences trigger a full review of that locale.
- **Screenshot review.** Playwright captures each enabled locale at 390 px and at desktop width: compose, accounts, post
  detail with its dialogs, MCP connections and sign-in. The `ui-reviewer` role checks clipping, wrapping, truncated buttons
  and mixed languages. Arabic, when added, is captured in both directions.
- **Live check.** Before a release, someone fluent in the locale runs the acceptance scenario once in that language.

## 5. Runtime: a client-side provider with an ICU subset (D-021)

> **Update (D-021).** next-intl was evaluated and supports Next 15, but its FormatJS formatter adds about 14 kB gzipped to every
> route, which would break the 110 kB shared-JS budget. The dashboard uses `frontend/src/i18n/` instead: the same ICU syntax,
> the same `useTranslations('ns')` call shape, about 2 kB. The notes below describe the original next-intl evaluation and still
> hold for the provider layout, the no-routing mode and the typed keys.

**Original choice: `next-intl`** for the dashboard.

- It is built for the App Router and works in the client components this codebase uses everywhere (`'use client'` views).
  It also works in server code (`getTranslations` for `metadata` titles).
- It supports "no i18n routing" mode. The dashboard sits behind a login and needs no `/ru/` URLs: the locale comes from the
  user's setting. Mount `NextIntlClientProvider` inside the existing `PrefsProvider` and load `messages/{locale}.json`
  with a dynamic import. That works the same in the standalone server build and in the static `output: 'export'` demo,
  which has no middleware. The one exception: server-rendered `metadata` titles stay English in the demo, because it has
  no request at render time.
- It uses ICU MessageFormat (FormatJS `intl-messageformat`), the same syntax as the catalogs and the checks above. There is
  one syntax everywhere.
- Its typed keys come from `en.json`, so missing keys fail `make lint`.
- `useFormatter` takes a global `timeZone` and `now`. It replaces the hardcoded `en-GB` and `en` Intl calls in
  `lib/time.ts` and `lib/calendar.ts`.
- It is small, MIT-licensed and actively maintained.

Alternatives considered:

- `react-intl`: the same ICU engine, but no App Router helpers. It is the fallback if next-intl stalls.
- `i18next`: its own plural-suffix format, with ICU only through a plugin. That means two syntaxes.
- Lingui: needs an SWC plugin pinned to Next's SWC version, which is brittle across upgrades.
- Paraglide: compiled and small, but its message format is not ICU.
- Next's built-in i18n routing: Pages Router only.

Record the choice in DECISIONS.md.

## 6. The other surfaces

- **Landing page** (`site/build.mjs`): format `site/i18n/{locale}.json` with `intl-messageformat` at build time. Write
  `/{locale}/index.html` with `hreflang` alternates, and keep the English page at `/`. Screenshots stay English with
  translated captions until the demo can switch locale.
- **Emails**: add `users.locale` (BCP 47, nullable, additive migration). Subjects and bodies come from
  `mail/messages/{locale}.json`. Go has no maintained full ICU MessageFormat library, and mail needs only named
  placeholders plus one plural (link lifetime). So keep `text/template` and `html/template`, pick plural categories with
  `golang.org/x/text/feature/plural`, and pass the duration as a number (render.go:33 passes the English string "48
  hours" today). Each locale gets golden tests, like the existing `testdata/*.golden`.
- **API errors**: keep `code` and the English `message`. Add an optional `params` object (`{"network":"linkedin","limit":3000}`)
  to the error body, and map `code` plus `fields` to catalog keys in `lib/api.ts`. The server message is shown only for an
  unknown code. This changes the contract: update ARCHITECTURE and the demo mock in the same PR.
- **Capability notes** (`capabilities.notes`, English text from the backend): add a stable `notes_key` per provider. The UI
  translates the key and falls back to `notes`.
- **MCP**: no translation. Tool descriptions, hints and `CONFIRMATION_REQUIRED` stay English (languages.md).

## 7. Rollout (one PR per step)

1. next-intl, `en.json` and `meta.json` for the shell, auth and shared states; `i18n:check`; the `en-XA` pseudo-locale.
2. Extract strings area by area (dashboard, composer, posts, accounts, developer), applying en-rewrite.md as each area
   moves. Each PR stays under about 400 lines of non-test code.
3. Locale-aware formatting (`useFormatter`) and the "Time format" setting.
4. `users.locale`, the language switcher and `navigator.languages` matching (uk never falls back to ru).
5. Catalogs in the D-021 order (languages.md): machine draft → back-translation sample → screenshots → enable with the "Beta translation" label → native review.
6. Localized emails, then the localized landing page.
7. `ar`, adding RTL support (`ar-XB` pseudo-locale and logical CSS) before `ar`.

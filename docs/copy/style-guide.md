# Steerpost copy style guide

This guide covers UI, emails, the landing page, docs, MCP tool descriptions and API error messages. Terms come from
[glossary.csv](glossary.csv): one term per concept in UI, API and docs (AGENTS.md section 7). When this guide and the
glossary disagree, the glossary wins for a term and this guide wins for everything else.

## 1. Voice

Steerpost sounds like a careful senior engineer who respects your time.

| Trait | Do | Don't |
|---|---|---|
| Clear | "Telegram rejected the image: photos are limited to 10 MB." | "Something went wrong with your media." |
| Calm | "Post scheduled." | "Woohoo! Your post is scheduled! 🎉" |
| Competent | "Retried 3 times. LinkedIn is still returning errors." | "We're on it!" |
| Honest | "Instagram is not available yet: it needs a Meta review." | "Instagram coming soon!" |

- Address the reader as **you**. The product is **Steerpost**, not "we". Exception: emails and docs may say "we" for the
  maintainers, and nowhere else.
- Name the actor. "Claude Desktop scheduled this post" beats "This post was scheduled". Agent actions must always be
  traceable to an agent.
- Promise only what works. A network that needs approval, a paid API or a live check says so in the same sentence (see
  PLATFORMS.md). Never write "coming soon" without a tracked issue.
- No hype or exclamation marks. Zero exclamation marks in product UI and emails.

## 2. Tone by situation

| Situation | Tone | Example |
|---|---|---|
| Success | short, past tense, no praise | "Draft saved" |
| Something is running | present progressive plus an ellipsis | "Publishing…" |
| Error the user can fix | neutral, then the fix | "Password is too short. Use at least 8 characters." |
| Error the user cannot fix | own it, then give a next step | "Steerpost could not reach LinkedIn. It will retry in 2 minutes." |
| Dangerous action | precise about consequences | "This posts to LinkedIn and Telegram now. Steerpost cannot undo it." |
| Empty | helpful, one next step | "No drafts yet. Write a post" |
| Agent activity | factual and attributable | "Claude Desktop called `publish_post` (confirmed)" |

## 3. Mechanics (English source)

- **US English** spelling: customize, color, canceled. The API status `cancelled` is a code identifier and stays as it
  is. UI copy today mixes in "customised" and "recognised"; fix them when you touch them.
- **Sentence case everywhere**: titles, buttons, tabs, nav, badges, menus and email subjects. Proper nouns keep their own
  case (LinkedIn, Dev.to, VK, Steerpost).
- Short sentences: aim for 15 words or fewer, never more than 25. One idea per sentence.
- **Contractions**: positive ones are fine ("you'll", "it's"). Write negatives in full ("cannot", "do not"); they are read
  and translated more reliably.
- **Punctuation**: full sentences end with a period. Buttons, labels, headings, tabs, badges and fragment toasts ("Draft
  saved") have none. No serial comma unless it prevents ambiguity. Use "…" (one character) for progress, never "...".
  Use curly quotes “ ” in UI text and straight quotes in code.
- Write "for example" in body text. Use "e.g." only in hints of 6 words or fewer. Never write "etc.", "via" or "i.e.";
  use "through" or "that is".
- Device-neutral verbs: **select** or **choose**, not "click", "tap" or "hit".
- Code identifiers (`posts:publish`, `confirm: true`, `PROVIDER_NOT_AVAILABLE`) go in code font. They are never
  translated, capitalized or pluralized.
- Every string is a whole sentence in the catalog. Never concatenate fragments ("Connected " + name + " successfully").
  Never add "s" for plurals: use ICU `plural` (see translation-process.md).

## 4. Buttons and links

- **Verb first, 1–3 words**: "Save draft", "Schedule", "Publish now", "Connect account", "Revoke key".
- Name the object when the screen has more than one: "Delete post", not "Delete".
- **A confirm button repeats the verb of its dialog title.** "Delete this post?" → "Delete post".
- The dismiss button is "Cancel", unless the action itself is a cancel. Then use "Keep post" to avoid two "Cancel"
  buttons side by side.
- A progress label uses the same verb: "Publish now" → "Publishing…".
- Never write "OK", "Yes", "No", "Submit", "Click here", "Go" or "Done" when a specific verb exists.
- "Try again" reloads data. "Retry" republishes to failed accounts. They are different keys and never swap.
- A link names its destination: "View on LinkedIn", not "View on platform" or "here".

## 5. Errors

Pattern: **What happened. What to do.** One or two sentences. The request reference goes at the end: "(ref 7f3a…)".

| Rule | Bad | Good |
|---|---|---|
| Say what happened, in user terms | "INVALID_STATE_TRANSITION" | "This post is already published, so it cannot be edited." |
| Give the fix | "Not found." | "This post no longer exists. It may have been deleted." |
| State the rule, not "invalid" | "Invalid password" | "Use 8–128 characters." |
| Name the network and the account | "linkedin account @x needs to be reconnected" | "LinkedIn account Abzal S. needs reconnecting. Reconnect it in Accounts." |
| Do not blame the user | "You entered a wrong date" | "Choose a time at least 1 minute from now." |
| No raw status or code words in sentences | "connection is expired" | "needs reconnecting" |
| Error ≠ empty | an empty list after a failed load | "Loading failed. Try again" |

- Field errors go under the field and say the rule. Form-level errors go above the buttons.
- Errors from a network carry the network's name and its reason in plain words, when the reason is safe to show.
- Avoid "Oops", "Sorry" (unless Steerpost is at fault), "Please" as filler and "Something went wrong" with no next step.

## 6. Empty states

Structure: **title** (the state, 5 words or fewer) + **one sentence** (what will appear here, or why it is empty) + **one
primary action**.

- First run: "No posts yet" / "Posts you write or schedule appear here." / [Write a post]
- Filtered: "No posts match these filters" / [Clear filters]
- Blocked by a missing prerequisite: "No accounts yet" / "Connect an account to write your first post." / [Connect account]
- Waiting on a network: "No analytics yet" / "None of your connected networks report analytics through their APIs."
- Never show an empty state for a failed load (section 5).

## 7. Confirming dangerous actions

These need a confirmation dialog (AGENTS.md section 7): **publish now**, **delete** a post, media or the account,
**disconnect** an account, **revoke** a key or MCP connection, **cancel** a post, and **grant a sensitive or critical
permission**.

1. **Title**: a question with verb + object. "Disconnect LinkedIn account?" Never "Are you sure?"
2. **Body** answers three things in 1–2 sentences:
   - what happens and to what (name the account, post or key);
   - whether it can be undone;
   - what stays (for example, published copies stay on the network).
3. **Confirm button**: the same verb, in danger style. **Dismiss**: "Cancel" (or "Keep …").
4. Irreversible and wide in scope (deleting the Steerpost account, revoking a key that agents use) → the user types the
   name or email to confirm.
5. For agents: MCP tools require `confirm: true`, and tool descriptions say "only after the user approved this exact
   action". Until server-side approval ships (ROADMAP goal 5), UI copy must not imply that Steerpost checks the human's
   answer. Write "The agent must send a confirmation flag", not "You will be asked to approve".
6. Approval copy for later: "{agent} wants to publish “{post}” to {accounts} now." [Approve] [Reject]

## 8. Numbers

- Format with `Intl.NumberFormat` in the UI locale, never `toLocaleString()` without a locale. Grouping varies: 1,234,567
  (en), 12,34,567 (en-IN), 1 234 567 (ru, uk, kk, fr), 1.234.567 (de, es, pt-BR).
- Units through `Intl.NumberFormat` `style: "unit"`: "10 MB" (en), "10 МБ" (ru). File limits and character counts are
  numbers, not words.
- Character counters: "{used} / {max}". Screen-reader text spells it out: "{used} of {max} characters".
- Write numerals in UI: "1 minute", "3 attempts". Spell out numbers only at the start of a sentence.
- Plurals go through ICU `plural` in every language, including English: never `attempt{n === 1 ? '' : 's'}`.
- Arabic uses Latin digits (`-u-nu-latn`) for counts, IDs and limits.

## 9. Dates and times

- Format with `Intl.DateTimeFormat` using the **UI locale** and the **user's time zone**. Do not hardcode `en-GB`, as
  `lib/time.ts` does today. Never build dates from parts.
- The clock follows the locale by default (en-US and en-IN use 12h; ru, uk, kk, de, es, pt-BR and ja use 24h). Settings
  gets "Time format: Automatic / 24-hour / 12-hour". A 12h time always shows AM/PM.
- Every scheduled time shows its time zone: "Oct 9, 2026, 2:30 PM (Asia/Almaty)". Write "time zone" as two words.
- Past events in the last 7 days are relative ("3 hours ago"); older ones are absolute. **Future publish times are always
  absolute**, with an optional relative hint: "Oct 9, 14:30 · in 2 days".
- Week start comes from the locale (Monday: en-GB, ru, uk, kk, de, es. Sunday: en-US, pt-BR, ja).
- No ambiguous numeric dates in text (03/04). Use medium style with a month name.
- APIs, MCP, logs and exports use ISO 8601 / RFC 3339 in UTC.

What `Intl` produces for 2026-10-09 09:30 UTC in Asia/Almaty, `dateStyle: medium, timeStyle: short`:

| Locale | Output | Clock | Week starts |
|---|---|---|---|
| en-US | Oct 9, 2026, 2:30 PM | 12h | Sun |
| en-GB | 9 Oct 2026, 14:30 | 24h | Mon |
| ru | 9 окт. 2026 г., 14:30 | 24h | Mon |
| uk | 9 жовт. 2026 р., 14:30 | 24h | Mon |
| kk | 2026 ж. 09 қаз., 14:30 | 24h | Mon |
| es | 9 oct 2026, 14:30 | 24h | Mon |
| pt-BR | 9 de out. de 2026, 14:30 | 24h | Sun |
| de | 09.10.2026, 14:30 | 24h | Mon |
| ja | 2026/10/09 14:30 | 24h | Sun |

Note: CLDR prints the kk unit as "10 MБ", with a Latin M and a Cyrillic Б. Check unit output in kk QA.

## 10. Capitalization

- Sentence case for all UI and email subjects ("Create API key", not "Create API Key").
- Keep official casing: Steerpost, LinkedIn, Telegram, Bluesky, Mastodon, Misskey, Dev.to, VK, X, Discord, Slack, MCP,
  OAuth, API, JSON, Claude Desktop, Claude Code, Cursor.
- Never use ALL CAPS for emphasis, including MCP descriptions ("SENSITIVE:"). The `[risk: …]` prefix already carries it.
- Badges and statuses are in sentence case too: "Partially published", "Needs reconnecting".

## 11. Words to avoid

| Avoid | Use | Why |
|---|---|---|
| simply, just, easy, seamless, effortless | (delete) | condescending, untranslatable |
| powerful, magic, revolutionary, AI-powered (as hype) | say what it does | hype |
| guaranteed, never fails, 100 %, instantly, unlimited | the real limit | overpromise |
| coming soon | "not available yet" + reason | honesty |
| Oops, Whoops, Nice, Awesome, Yay | (delete) | tone |
| click, tap, hit | select, choose | device-neutral |
| platform, provider (in UI) | network | one term (provider stays in the API) |
| target (in UI) | account | user term (target stays in the API) |
| universal content | main text | jargon |
| native scheduling | scheduled by the network | reads as "no scheduling" |
| needs review (post status) | unconfirmed | clashes with approvals |
| stub, mock (in UI) | not available yet / test network | internal |
| leash, revive, on the fly, out of the box | plain words | idioms do not translate |
| invalid, illegal, fatal, abort, kill, execute | say the rule; stop; run | harsh or vague |
| whitelist, blacklist, master/slave, sanity check | allow list, block list, primary/replica, quick check | inclusive |
| customise, behaviour, recognised | customize, behavior, recognized | US English |

## 12. Global readiness (all languages)

- No idioms, puns, sports metaphors or culture-bound jokes. Examples use neutral names and topics.
- Do not refer to people with gendered pronouns. Write "they", or restructure.
- Design for +35 % text length (de, ru, es) and for CJK width. Buttons never truncate, and labels wrap rather than clip.
- Never put text in images. Screenshots on the landing page are per-locale assets or keep English UI with a caption.

## 13. Per-language decisions

| | Address | Buttons | Body text | Quotes | Notes |
|---|---|---|---|---|---|
| **ru** | «вы», lowercase, including emails | infinitive: «Сохранить», «Опубликовать» | polite imperative: «Подключите аккаунт.» | «…», nested „…“ | Write ё. Em dash with spaces ( — ). Avoid gendered past tense about people or agents: «Действие: Abzal», not «Abzal опубликовал». Use established Russian over slang («запланировать», not «зашедулить»). |
| **uk** | «ви», lowercase | infinitive: «Зберегти», «Опублікувати» | «Підключіть обліковий запис.» | «…» | Translate from en, **never from ru** (no calques or surzhyk). Apostrophe ’ (U+2019). Terms: допис, чернетка, обліковий запис. |
| **kk** | «сіз», lowercase | verbal noun in -у/-ю: «Сақтау», «Жариялау», «Жою» | «Аккаунтты қосыңыз.» | «…» | Cyrillic script. No case suffix on placeholders (vowel harmony). Established loans are fine: аккаунт, медиа, API. All strings need a native reviewer. |
| **es** | **tú** (the norm for developer tools) | infinitive: «Guardar», «Programar» | «Conecta una cuenta.» | “…” | Neutral international Spanish (es-419 vocabulary, no vosotros, no regionalisms). Use opening ¿ ¡ marks, but never exclamations. |
| **pt-BR** | **você** | infinitive: «Salvar», «Agendar» | «Conecte uma conta.» | “…” | Brazilian forms only: tela, arquivo, usuário (not ecrã, ficheiro, utilizador). "post" is the accepted term. |
| **de** | **du**, lowercase (the norm for developer and SaaS tools in German) | infinitive: «Speichern», «Planen» | «Verbinde ein Konto.» | „…“ | Nouns capitalized by grammar. Address the reader as "du" to avoid gendered nouns like "Nutzer". Prefer short verbs to keep within the length budget. |
| **ja** | no pronoun (never あなた); です/ます sentences | noun or short verb: 「保存」「今すぐ投稿」「削除」 | 「アカウントを連携してください。」 | 「…」 | Full-width 。、, half-width letters and digits, no space between ja and Latin (JTF style). Katakana long-vowel mark: サーバー, ユーザー. Error pattern: 〜できませんでした。〜してください。 |
| **fr** | **vous** | infinitive: «Enregistrer», «Programmer» | «Connectez un compte.» | « … » with narrow no-break spaces | Space before `:`, `;`, `?`, `!` is a narrow no-break space (U+202F), never `!` in UI copy. Long words: check the length budget. Draft: needs a native pass. |
| **id** | **Anda**, capitalized | base verb: «Simpan», «Jadwalkan» | «Hubungkan akun.» | “…” | No plurals; repeat the noun for emphasis only when needed. Use «akun», «unggah», «pos» (established loans). Draft: needs a native pass. |
| **zh-CN** | no pronoun; 你 only when unavoidable | verb or short verb-object: 「保存」「立即发布」 | 请连接账号。 | 「」 not used; use “…” | Simplified Chinese only. Full-width punctuation, no spaces between Chinese and Latin or digits is acceptable but be consistent. No `!`. Do not claim "works with Claude". Draft: needs a native pass. |
| **ar** | formal second person, gender-neutral where possible | verbal noun: «حفظ», «جدولة» | «اربط حسابًا.» | «…» | Modern Standard Arabic. Latin digits (`ar-u-nu-latn`). All six plural categories. Wrap placeholders in bidi isolation. Draft: needs a native pass, last in the rollout. |

## 14. Do not translate

| Term | Rule |
|---|---|
| Steerpost | never translated, transliterated or declined with an attached ending (kk, tr: restructure the sentence) |
| MCP, API, REST, OAuth, OAuth 2.1, JSON, HTTP, stdio, CSRF | always Latin, as is |
| webhook | Latin in every language (de capitalizes it as a noun: "Webhook") |
| API key | keep **API** in Latin. Translate **key** with the fixed glossary word: «API-ключ», «API-Schlüssel», 「APIキー」, «clave de API». An English "key" inside a Russian sentence fails the no-English check and reads as a bug; all major developer products translate it. |
| Network names | LinkedIn, Telegram, Bluesky, Mastodon, Misskey, Discord, Slack, Dev.to, VK, X, Threads, Instagram, Facebook, YouTube, TikTok, Pinterest, Tumblr, Nostr, WordPress, Ghost, Max. ru marketing copy may write «ВКонтакте (VK)» on first mention; the UI uses VK. |
| Product and client names | Claude, Claude Desktop, Claude Code, Cursor, GitHub, Docker |
| Code | tool names (`publish_post`), scopes (`posts:publish`), API statuses (`needs_review`), error codes, env vars, `confirm: true`, `sk_live_…` |

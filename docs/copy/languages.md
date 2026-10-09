# Which languages SocialOS supports, and in what order (draft 2026-10-09)

Legend: **[S]** comes from secondary sources (listed at the end). **[U]** has not been verified. Network tiers come from
[PLATFORMS](../PLATFORMS.md).

## Decision

| Wave | Languages | When |
|---|---|---|
| **1 (launch, 8)** | `en` (source), `ru`, `uk`, `kk`, `es`, `pt-BR`, `de`, `ja` | UI and emails at launch. The landing page gets the hero and the networks table in each language. Docs stay English. |
| **2 (5)** | `fr`, `id`, `tr`, `ko`, then `ar` | `fr`, `id` and `tr` come right after launch. `ko` comes with Threads (Tier 2). `ar` comes once the RTL work below is done. |
| Not planned | `zh-CN`, `hi` | Revisit when the triggers below are met. |

Covered by `en` until then: India, Nigeria, the Philippines and the Nordics. Developers there mostly use English UIs.

## How languages were ranked

1. **Network fit.** Do people in that market use the networks SocialOS can publish to: live (LinkedIn, Telegram), Tier 1
   (Discord, Mastodon, Bluesky, Slack, Dev.to, VK, Misskey) or Tier 2 (Threads, Instagram, Facebook, Tumblr)? A language
   whose main networks are Tier 3 or blocked brings few active users.
2. **Developer and AI-agent audience.** GitHub developer counts and Claude usage by country.
3. **Need for a translated UI.** Where developers read English comfortably (the Nordics, the Netherlands, India), translation
   adds little. Where they do not (Japan, Brazil, Latin America, Russia, Turkey, Indonesia), it decides whether people adopt.
4. **Cost and risk.** Script and plural complexity, and whether a native reviewer is available.

## Ranking

| Lang | Main markets | Network fit | Dev / AI audience | UI need | Cost | Wave |
|---|---|---|---|---|---|---|
| en | global | all | US is #1 on GitHub and #1 in Claude usage [S] | — | source | 1 |
| ru | RU, KZ, UZ, KG, BY | **Telegram** (#2 market) + **VK** (Tier 1). LinkedIn is blocked in RU | large; the owner is a native speaker | high | low (plurals as uk) | 1 |
| uk | UA | **Telegram** (main news channel), LinkedIn (outsourcing sector), Discord | large outsourcing developer base | medium | low; translate from en, never from ru | 1 |
| kk | KZ (home market) | **Telegram** (#1 messenger), VK. Instagram (Tier 2) matters for local business | small but local | medium (state language; tech users are often bilingual with ru) | needs a native reviewer; script will change (see below) | 1 |
| es | MX, ES, CO, AR, CL | LinkedIn, Telegram (MX ~34 % use it [S]), Discord, Bluesky/Mastodon in ES | large and growing LatAm developer base | high | low | 1 |
| pt-BR | BR | **Bluesky** (US, JP and BR are its top 3 [S]), **Telegram** (BR ~38 % use it [S]), LinkedIn, Discord | #4 on GitHub, grew more than 4× in 5 years [S]; top 5 in Claude usage [S] | high | low | 1 |
| de | DE, AT, CH | **Mastodon** (its largest market [S/U]), Bluesky (~5 % of traffic [S]), LinkedIn | large; strong demand for self-hosting and privacy | medium | low (long words: see the length budget) | 1 |
| ja | JP | **Bluesky** (top 3 [S]), **Misskey** (from Japan, Tier 1), Mastodon. X dominates but is Tier 3 (paid) | large; top 5 in Claude usage [S] | very high | **CJK work** | 1 |
| fr | FR, BE, CA-QC, francophone Africa | Mastodon, Bluesky, LinkedIn | large | medium | low | 2 |
| id | ID | **Telegram** (#3 market [S]), LinkedIn, Discord | grew more than 4× on GitHub [S] | high | lowest: no plurals, Latin script | 2 |
| tr | TR | Telegram, LinkedIn, Bluesky (spikes during X blocks) | large | high | medium: suffixes after placeholders | 2 |
| ko | KR | Threads (Tier 2), Discord. Kakao and Naver are not supported | top 5 in Claude usage [S] | high | CJK work, particles after placeholders | 2 (with Threads) |
| ar | EG, SA, AE, MA | Telegram, LinkedIn (Gulf) | growing | high | **RTL work** | 2 (last) |
| zh-CN | CN | Telegram, LinkedIn, Discord, X and Bluesky are blocked in mainland China; Claude is not offered there [U] | huge but cannot use the product | — | CJK | revisit when WeChat or Weibo appear in PLATFORMS |
| hi | IN | Telegram (#1 market [S]), LinkedIn | huge, but developers there use English UIs | low | Devanagari fonts | revisit when non-developer users ask for it |

Why `kk` is in wave 1 although its market is small: the owner is in Kazakhstan, Telegram is the main channel there,
and a Kazakh UI earns local trust and partners (Astana Hub, local agencies). The value grows when Instagram (Tier 2) lands.
`uz` (Uzbek, Latin script) is a wave-3 candidate for the same reason: Telegram dominates there. Until then, `ru` covers
many Uzbek users.

Why `uk` is in wave 1 next to `ru`: Ukrainian developers are a large Telegram and LinkedIn audience. Offering `ru` without
`uk` would push them to Russian. The rules below make sure a Ukrainian browser never falls back to Russian.

Claude is not offered in Russia or mainland China [U]. MCP works with any client, so the `ru` audience can use other MCP
agents. Do not promise "works with Claude" on pages localized for those markets.

## Networks by market

| Network (status) | Strongest markets | Languages it supports |
|---|---|---|
| Telegram (live) | IN, RU, ID, US, BR, UA, KZ, UZ, IR [S] | ru, uk, kk, pt-BR, id, es, hi |
| LinkedIn (live, personal profiles) | US, IN, BR, UK, FR, DE, MX; blocked in RU | en, pt-BR, es, fr, de |
| VK (Tier 1, needs a live check) | RU, BY, KZ | ru, kk |
| Bluesky (Tier 1) | US, JP, BR, UK, DE, CA [S] | en, ja, pt-BR, de |
| Mastodon / Misskey (Tier 1) | DE, FR, JP, US, UK [S/U] | de, fr, ja, en |
| Discord, Slack (Tier 1) | global developer communities | all |
| Dev.to (Tier 1) | English-language developer blogs | en |
| Threads, Instagram (Tier 2, owner only) | IN, US, BR, ID, MX, KR, KZ (Instagram drives business there) | pt-BR, es, ko, kk |

## Extra engineering per language

**RTL (`ar`, later `he`, `fa`).**
- Set `dir="rtl"` on `<html>`. Replace physical Tailwind utilities with logical ones: the code today uses `ml-*`, `mr-*`,
  `pl-*`, `left-*`, `right-*`, `text-left`, `text-right` and `md:border-r`; switch to `ms-*`, `me-*`, `ps-*`, `start-*`,
  `end-*`, `text-start`, `text-end` and `md:border-e`. Add a lint rule.
- Mirror directional icons: the calendar's Previous/Next chevrons and the external-link arrow.
- Wrap every placeholder in bidi isolation (`<bdi>` or U+2068…U+2069), so Latin handles and URLs do not flip the sentence.
- Show user content with `dir="auto"`. Posts can be Arabic inside an English UI, and the other way round.
- Pin `ar-u-nu-latn-ca-gregory`. Some engines default `ar-SA` to the Islamic calendar, and Arabic-Indic digits break IDs,
  counts and code.
- Inter has no Arabic glyphs. Add a fallback (Noto Sans Arabic or IBM Plex Sans Arabic) and more line height.
- Effort: about one sprint. Screenshots at 390 px in both directions are required.

**CJK (`ja` now, `ko`/`zh` later).**
- Inter has no CJK glyphs. Use the system stack (`"Hiragino Sans", "Yu Gothic UI", "Meiryo", "Noto Sans JP", sans-serif`)
  and ship no CJK web font: it would add several MB.
- Line breaking: `line-break: strict` and `word-break: auto-phrase` (Chromium) for ja. Keep `overflow-wrap: anywhere` for
  long Latin tokens such as keys and URLs.
- Turn off `uppercase` and `tracking-*` for CJK: the scope picker's group labels use both. No italics, no synthetic bold.
- Composer hotkeys must ignore keys while `event.isComposing` is true, so IME input is never submitted half-typed.
- Length budgets count display width, not characters: one CJK glyph is about two Latin ones.
- Week start and date order come from `Intl` (ja: Sunday, `2026/10/09`). Never build them by hand.

**Plural rules (CLDR, as Node 25 / ICU report them).**

| Lang | Categories | Watch out for |
|---|---|---|
| en, de, tr, hi | one, other | — |
| **ru, uk** | one, few, many, other | 1 пост / 2 поста / 5 постов / 1,5 поста. `other` is for fractions only, but it must exist. |
| **kk** | one, other | After a number the noun stays singular (5 жазба, not 5 жазбалар), so both forms often match. The verb ending still agrees. |
| es, pt, fr | one, many, other | `many` is used only for exact millions ("1 millón de publicaciones"). It is still required. |
| ja, ko, zh, id | other | Write the counter word in the string (`{count}件`). |
| **ar** | zero, one, two, few, many, other | All six are required. `zero` and `two` have their own grammar. |

**Words attached to placeholders.**
- kk and tr change suffixes by vowel harmony (`LinkedIn-ге` vs `Telegram-ға`). ko changes particles (`이/가`). ru, uk and kk
  decline nouns.
- Rule for every language: put a placeholder where no ending attaches to it ("Network: {network}"), or put it after a
  noun that takes the ending ("аккаунт {name}").

## Locale tags, detection and fallback

- Tags (BCP 47): `en`, `ru`, `uk`, `kk` (Cyrillic today, `kk-Cyrl`), `es` (neutral international Spanish), `pt-BR`, `de`,
  `ja`.
- Kazakhstan plans to move Kazakh to the Latin script by 2031. Keep `kk` script-tagged internally, so `kk-Latn` can be added
  later without renaming keys.
- First visit: the best match of `navigator.languages` or `Accept-Language`. `pt-PT` → `pt-BR`. `es-*` → `es`. `de-AT` and
  `de-CH` → `de`.
- Every locale falls back to `en`. **`uk` never falls back to `ru`.** `kk` falls back to `en`, not `ru`. Mixed-language
  screens are avoided anyway, because CI blocks missing keys.
- A signed-in user's choice is stored on the profile (new `users.locale`) and used for emails. Before sign-in it lives in a
  cookie.
- The switcher shows endonyms without flags: English, Русский, Українська, Қазақша, Español, Português (Brasil),
  Deutsch, 日本語.

## What stays English in every wave

- MCP tool names, descriptions, parameter docs and error hints. Their reader is a model, and English is part of the contract.
  The agent relays the result in the user's language.
- API error `code` and the English `message` (for API consumers). The UI shows its own translated message for each `code`.
- Code identifiers: scopes (`posts:publish`), statuses in the API (`needs_review`), tool names and env vars.
- Docs (`site/docs/*`) until wave 2. The landing page is translated selectively, as above.

## Sources (accessed 2026-10-09)

- GitHub Octoverse 2025 coverage: [It's FOSS](https://itsfoss.com/github-octoverse-2025/),
  [YourStory](https://yourstory.com/ai-story/github-india-worlds-largest-developer-hub),
  [OpenSourceForU](https://www.opensourceforu.com/?p=91285),
  [Second Talent table](https://www.secondtalent.com/resources/countries-with-most-software-developers/)
- Claude usage by country: [Anthropic Economic Index, geography](https://www.anthropic.com/news/economic-index-geography),
  [How Australia uses Claude](https://www.anthropic.com/research/how-australia-uses-claude),
  [IT Brief](https://itbrief.com.au/story/australia-tops-claude-usage-per-capita-anthropic-says)
- Bluesky traffic by country: [Sprout Social](https://sproutsocial.com/insights/bluesky-statistics/),
  [Backlinko](https://backlinko.com/bluesky-statistics), [Proxidize](https://proxidize.com/blog/bluesky-user-count-2026/)
- Telegram by country: [BankMyCell](https://bankmycell.com/blog/number-of-telegram-users),
  [TechRT](https://techrt.com/telegram-statistics/), [Statista topic](https://www.statista.com/topics/9640/telegram/)
- Mastodon by country (weak data): [Marketful](https://marketful.com/mastodon-statistics),
  [wmtips](https://www.wmtips.com/technologies/social-media/mastodon/)
- Plural categories, date and number samples: `Intl.PluralRules` and `Intl.DateTimeFormat` in Node 25.6 (CLDR as bundled).

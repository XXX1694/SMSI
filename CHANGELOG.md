# Changelog

All notable changes to Steerpost are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and versions follow [Semantic Versioning](https://semver.org/). Pushing a tag `vX.Y.Z` builds the images and publishes a
GitHub Release whose notes are the matching section of this file (see "Releasing" in [`deploy/README.md`](deploy/README.md)).

## [Unreleased]

### Added

- Russian and Kazakh catalogs for the dashboard and the demo (`frontend/messages/ru.json`, `kk.json`, 687 keys each, ICU plurals per CLDR). Machine drafts for native review: neither locale is enabled yet (separate change), `docs/copy/review/{ru,kk}.md` list the terms and strings to check. The post-list row wraps its retry button and fits longer Edit labels at 320 px.
- App translations for es, pt-BR, de, fr and id (D-021): complete catalogs in `frontend/messages/<locale>.json` (676 keys each, CLDR plural categories, glossary terms), machine drafts with the choices and open questions in `docs/copy/review/<locale>.md` (section "App"). The locales are not enabled yet, so the language switcher is unchanged.
- Japanese (`ja`) and Simplified Chinese (`zh-CN`) catalogs for the dashboard and the demo: all 676 keys in `frontend/messages/ja.json` and `zh-CN.json`, machine drafts that follow `docs/copy/glossary.csv` and the landing translations (CJK punctuation, `other`-only plurals, no "Claude" claim in zh-CN copy). Neither locale is enabled yet (`frontend/src/i18n/locales.ts` is unchanged), so users still see English until a separate change enables them. `docs/copy/review/ja.md` and `zh-CN.md` have an "App" section with the terminology and the strings that need a native check. The audit-log filter buttons wrap on narrow screens.
- Landing page in 9 languages (D-021): ru, es, pt-BR, de, fr, id, ja, zh-CN and ar, besides English, at `/{locale}/` (English stays at `/`, `/en/` forwards to it). `site/build.mjs` renders the page from `site/i18n/landing.<locale>.json` (ICU plurals and rich tags through `intl-messageformat`), with `<html lang dir>`, localized title, description and Open Graph tags, hreflang for every locale plus `x-default`, and the locales in the sitemap. Docs and the demo stay English and are linked with "(EN)". Every translation is a machine draft labelled "Beta translation" in the footer; `docs/copy/review/<locale>.md` lists what needs a native look. `kk` is built hidden (`noindex`, not linked) until a native reviewer signs it.
- A compact language menu in the nav and a plain link list in the footer, both with endonyms, no flags, one line at 320 px. A first-visit suggestion on the English page follows `navigator.languages` (same matching as the app, `uk` and `zh-Hant` get English); it is a small dismissible card, never a redirect, and the choice is remembered in `localStorage` (`steerpost_locale`, shared with the demo).
- Right-to-left layout for `ar` through logical CSS properties; CJK and Arabic use system fonts with phrase-aware line breaking and no negative tracking; Cyrillic uses Inter's own Cyrillic subsets (fetched only by pages that need them). "Pause motion" and reduced motion behave the same in every language.
- `npm run check` verifies a page, a complete catalog and complete hreflang for every locale; `npm run smoke` loads each locale at 320, 390 and 1440 px and asserts no horizontal overflow, a one-line nav, the motion switch and the suggestion.
- Internationalization infrastructure (D-021; no translated copy yet). A client-side locale provider that works the same in the server build and the static demo (no middleware, no locale routes): the locale is resolved as user setting (later `users.locale`) → `localStorage` (`steerpost_locale`) → `navigator.languages` → English, `<html lang>` and `dir` follow it (right-to-left only for `ar`), and `uk`, `zh-TW`, `zh-HK` and `zh-Hant` browsers get English. Message catalogs live in `frontend/messages/<locale>.json` with `en` as the source and empty files for the target locales; a missing key falls back to English. ICU MessageFormat comes from a small in-house runtime (`src/i18n/icu.ts`, plural, select, ordinal, rich tags) with typed keys; it is checked against FormatJS in tests. next-intl 4.14 supports Next 15 but added about 14 kB gzipped to every route (measured: first-load JS of `/` 114.8 kB → 128.7 kB), so it was not used; the runtime adds about 2 kB (114.8 kB → 116.9 kB, the Next build table stays at 103 kB shared).
- Language switcher in Settings → Preferences and a compact one in the sidebar. It lists only enabled locales by their own name and marks machine-drafted ones "Beta translation". Only English is enabled so far; the pseudo-locale `en-XA` (accented, about 35 % longer) is listed in dev builds and the demo, to find hard-coded strings and overflow.
- `Intl` formatting helpers for numbers, dates, times and relative times that honour the locale and the Settings timezone (`src/i18n/format.ts`, `useFormat()`). Arabic keeps Latin digits and the Gregorian calendar.
- `npm run i18n:check` (part of `npm run lint`): every catalog key must exist in English, every message must be valid ICU with the same placeholders and tags, enabled locales must be complete with all plural categories, code may only use keys English defines; unused keys and missing `meta.json` descriptions are warnings.
- Every user-visible string of the dashboard and the demo now comes from the English catalog (`frontend/messages/en.json`, 674 keys in 17 namespaces (`nav`, `common`, `errors`, `shell`, `auth`, `dashboard`, `posts`, `composer`, `calendar`, `accounts`, `approvals`, `settings`, `developer`, `media`, `analytics`, `legal` and `language`). The English text is unchanged. Counts and states are ICU `plural` and `select` messages (no more string concatenation or hand-made "s"), dates, numbers, sizes and relative times follow the locale and the Settings time zone, and every key has a note for translators in `messages/meta.json`, including the ambiguous ones (Draft, Schedule, Expired, Never, Cancel vs Cancel post vs Keep post, Retry vs Try again, Connect, Requested by, Approved: action, "Post this code there", "{n} min left" and others).
- `npm run i18n:literals` (part of `npm run lint`, and `tests/i18n-literals.test.ts`) fails on hard-coded English in JSX text, in `aria-label`, `title`, `placeholder`, `alt` and the copy props of components, and in string literals that read like UI sentences. The allow-list (`frontend/scripts/i18n-literals.allow.json`) names a reason for every exception: the Terms and Privacy text, static metadata and the web manifest, the demo seed and engine, API fallback messages. `tests/i18n-extract.test.tsx` renders screens in `en-XA` and fails if plain English is left on screen.
- `<T k="..." />` for server components, `useErrorText()` and `nodes()` (rich messages with links) in `src/i18n`; `lib/*` functions take the translator as a parameter and stay pure.
- The sidebar navigation labels are the first strings read from the catalog.
- Reconnect button on accounts that need reconnecting (OAuth and token networks).
- Glossary: approvals, deny, trusted key, main text, operator, open-source and the approval statuses; `docs/copy/style-guide.md` section 7 describes server-side approval.
- App: a "Skip to content" link as the first tab stop and a focusable `main` landmark; failed posts on the dashboard get a Retry button; the calendar "+N more" and the mobile post count open that day.
- Data export (D-018): Settings, "Your data", builds a ZIP with your profile, posts with targets and attempts, connected accounts (no credentials), API key names, approvals, the audit log and your media files. The worker streams it to storage within its memory cap; the download link works for 5 minutes, the file is deleted after `EXPORT_RETENTION_DAYS` (default 7). One export at a time, one per 24 hours. New endpoints `POST/GET /account/exports` and `GET /account/exports/{id}` (browser session only), migration 00005 (indexes).
- Account deletion (D-019): Settings, "Your data", "Delete account". It asks for your password and your email typed out, signs out every session and revokes every API key and MCP connection at once, sets scheduled posts back to drafts, and deletes everything after `ACCOUNT_DELETION_GRACE_DAYS` (default 7). Until then you can sign in, see a banner and cancel. The worker removes posts, media (S3 objects too), connected accounts and their stored credentials, keys, approvals and the audit log in batches; only a record of counts without personal data stays. New endpoints `POST /account/delete` and `POST /account/delete/cancel` (browser session only), `user.deletion_scheduled_at` and `deletion_grace_days` in `/me`, an email when deletion is scheduled, migrations 00006 and 00007.
- `429 RATE_LIMITED` answers can carry a specific `Retry-After`.

### Fixed

- Worker shutdown: the publish and export servers stop taking tasks together and drain in parallel (the export server waits 5 s), so a stop fits the 45 s `stop_grace_period`. An export archive upload may now run as long as the build (90 minutes) instead of 30; a finished build can no longer overwrite an export the sweep already failed.
- Landing hero video: the cursor no longer jerks. The dot used the CSS `scale` property together with `transform: translate`, so every click scaled its position towards the page origin and it flew across the screen for a moment. It now uses `translate`, and the video is rendered frame by frame (virtual time, 60 fps, cursor path computed per frame) instead of screen-recorded at about 25 fps with dropped frames. The video and posters were re-recorded; the poster shows the whole compose form.
- i18n groundwork before the first real locale: sentences that were assembled from pieces are single ICU messages now (approvals badge "99+" with the real count for screen readers, "+N more on {date}" with a locale-formatted date, capability badges one sentence per state, the locked-post notice picked by status code); an empty network id reads "Unknown" everywhere (`providerName`, `useProviderName`) instead of "Text on " or an empty list; the dashboard no longer overflows 320 px in the pseudo-locale; the count-only approvals pill on mobile shows the Approvals icon. Dates in the approval summary follow the locale like everywhere else, the duplicate `formatDateTime` in `lib/time` is gone, and ESLint bans it, `providerLabel` in UI code, `toLocale*String` and hard-coded `en-GB`/`en-US`. The pseudo-locale test now covers the dashboard, composer, post detail, calendar, accounts, developer and settings screens.
- Plural forms in the UI come from ICU messages: "1 requests per minute" for agents is now "1 request per minute", and the same rule covers attempts, accounts, attachments and characters.
- A failed publishing attempt whose server message is only a code shows the sentence for the attempt's error code (before: the generic "Something went wrong").
- Outside English the UI never shows the server's English error text; it shows the sentence for the error code from the catalog (field-level messages of the token form are mapped to catalog keys).
- The site smoke test confirms the demo's Reset dialog (it had been failing since the reset confirmation landed).
- Pages no longer scroll slightly on notched iPhones: the safe-area body padding is now subtracted from the `min-h-screen` and `md:h-screen` heights (dynamic viewport units, so the mobile browser bars are accounted for); desktop and Android, with zero insets, are unchanged.
- The completed setup checklist collapses to a "Set up" line with a "Show steps" button; the Approvals tab is kept in the URL (`?tab=history`); stacked table rows (Developer on phones) are more compact.
- Calendar month view no longer scrolls sideways at 768 to 1280 px; event chips are at least 24 px tall with a gap between them.
- Approval cards no longer break labels on phones and show network and status names (LinkedIn, Telegram, Draft) instead of codes.
- Settings Terms and Privacy links are underlined, not only coloured. The Escape key closes the mobile menu and returns focus to its button; the mobile header stays visible while scrolling.
- Touch targets: buttons, nav links, pending pill, legal links, "View all" and the demo Reset reach 44 px (24 px for Reset) on phones while desktop stays compact. Toasts respect the bottom safe-area inset.
- Demo banner is a labelled landmark, and Reset asks before wiping demo data. Loading states carry visible-to-screen-reader text, the email notice close button and the dashboard "View all" links have specific names, and the dashboard stats row has a heading.
- "Canceled" is spelled the American way in the message about a post that cannot be edited (it said "cancelled"), matching the status name.
- The "requests waiting" pill in the mobile header no longer wraps: on phones it shows only the count (screen readers still hear "N requests wait for you"), and the full text appears from 640 px.

### Changed

- The Privacy Policy describes self-service export and account deletion, and how long data stays.
- Landing page v2: about half the words (hero: "AI agents draft posts. You stay in control."), a scroll-driven "how it works" route (the line draws as you scroll, the approval gate locks, posts fan out to the networks), word-by-word headline reveals, clip-path screen reveals, magnetic buttons, tilting screens and a cursor light (mouse only), and a slimmer mobile hero and story with 44 px tap targets and safe-area insets. Pause motion now stops scroll animations and reveals too and exposes `aria-pressed`. Fixes: the nav logo no longer shrinks at 320 px, the hero flow no longer runs under the text, smooth anchor scrolling.
- Steerpost is now open-source under the AGPL-3.0 (`LICENSE`, D-022).
- App copy follows the copy review (`docs/copy`): shorter and plainer text, one term per concept, no idioms, translator notes next to the 14 ambiguous strings. No API values changed.
- Honesty fixes: the MCP panel and the "I understand" checkbox on API keys describe approvals (D-013), not a confirm flag; Publish now and Retry now count as irreversible in Approvals (danger-style Approve); each unavailable network shows its own reason from the capabilities (X, Medium and Hashnode no longer say "Requires platform approval"); the token form promises HTTPS only when the page is served over HTTPS; the media delete warning matches the backend (a file used in a post cannot be deleted); the Terms say API-key dangerous actions need approval unless the key is trusted, and name the AGPL-3.0.
- Status names: the target status "Needs review" and the attempt status "Unknown outcome" are now "Unconfirmed" in the UI (API values `needs_review` and `unknown` are unchanged). "Cancelled" is "Canceled", "Expired" on an account is "Needs reconnecting" (with a Reconnect button), the test network is no longer called "Mock", and the post page says "Accounts" instead of "Targets".
- Backend messages that reach the UI or agents are plain sentences with a next step (no raw ids, statuses or byte counts). Provider notes drop internal words; unavailable networks say "Not available yet: <reason>".
- Mail templates: one verb ("Verify"), "server admin" instead of "operator", and the export mail no longer points to a settings page that has no export.
- MCP tool descriptions and error hints: no duplicate SENSITIVE/CRITICAL prefix, correct reconnect and scope guidance, `needs_review` explained, agents are told to show the final text, accounts and time before `schedule_post`.
- Docs: D-021 (locale set and rollout order, `uk` waits, `zh-CN` joins), `docs/copy/languages.md` and `translation-process.md` updated, a release is no longer blocked by a locale's review status (machine-drafted locales ship as "Beta translation"), and the copy glossary has `zh-CN`, `ar`, `fr` and `id` columns.
- All user-visible app strings now come from the message catalog, so the JavaScript that ships with the app grows: the English catalog (about 10.5 kB gzipped) lands in a shared chunk, so first-load JS on `/login` goes from 131 to 147 kB, `/verify-email` 125 to 140, `/compose` 155 to 168 and `/accounts` 149 to 162 (the shared baseline stays 103 kB). Splitting the catalog by namespace is a follow-up (D-021).
- File sizes above 999 KB group thousands in English ("1,024 MB" instead of "1024 MB"), as the number follows the locale.

## [0.3.0] - 2026-10-09

### Added

- Steerpost brand identity: new mark, wordmark and lockups (light, dark, `currentColor`), favicon and app icons, web manifest, social preview and README banners, a teal palette in `tokens.css` with the AA test extended to the brand pairs, `--ease-steer` and `--duration-path` motion tokens, the logo in the app sidebar, and a landing hero whose flow lanes steer through one approval gate. See `docs/BRAND.md`; copy is renamed separately.
- Edit drafts and scheduled posts in the web UI. "Edit" on the Posts list and on the post page opens the composer (`/compose?post=<id>`) prefilled with the title, text, per-network overrides, media and schedule (shown in the Settings timezone); saving calls `PATCH /posts/{id}`. A scheduled post keeps its time unless you change it; a draft can be saved and scheduled in one step. Other statuses show why they cannot be edited. Unsaved edits are guarded (browser prompt on reload or close, a dialog on in-app links). The API has no ETag or `If-Match`, so before saving the UI re-reads the post and, if its `updated_at` or status moved, shows a conflict message with "Load the latest version" and "Save mine anyway" (best effort: the check and the write are not atomic).
- The demo engine and `mock-api` answer `PATCH /posts/{id}` (text, per-network text, accounts, media, time), so the Pages demo supports editing.
- Dashboard "Get started" checklist for new users: connect a network, write a first post, connect an AI agent (an MCP connection or an API key) and, optionally, review an approval. Each step ticks off from existing data (accounts, posts, keys, connections, approvals), has a one-line explanation and a direct link, and the list can be dismissed (remembered in this browser). The empty Accounts, Posts and Developer screens link back to the setup steps. The demo reflects its own state.

### Changed

- Rename shims for Steerpost (D-020). The MCP server reads `STEERPOST_API_URL`, `STEERPOST_API_KEY` and `STEERPOST_TIMEOUT_MS` first and falls back to the `SOCIALOS_*` names; both compose files set both URL variables. Releases publish the images under `steerpost-{backend,mcp,frontend}` as well as `socialos-*` (same digest and tags). `deploy/README.md` lists the legacy identifiers that keep the `socialos` name and the steps to make the new GHCR packages public.
- `autoupdate.sh` follows redirects when it asks GitHub for the latest release (a renamed repository answers 301), reads `GITHUB_REPO` from the environment as well as `.env`, and logs a warning for any answer other than 200 instead of an info line.
- Renamed to Steerpost (formerly SocialOS). Product copy, the MCP server name and the generated client config key (`steerpost`) changed. Stored and host identifiers keep the `socialos` name (cookies, headers, Redis keys, `/opt/socialos`, systemd units); see "Legacy identifiers" in `deploy/README.md`. Generated stdio configs keep the `SOCIALOS_AUTH_HEADER` variable so configs users already pasted keep working.

## [0.2.1] - 2026-10-09

### Added

- Agent request limit `QUOTA_AGENT_RPM`: requests per minute for all API keys and MCP connections of a user together, answered with `429 RATE_LIMITED`. Off by default (`-1`), like the other plan limits (D-014).
- MCP tool `get_usage` (the 14th, scope `analytics:read`) shows the plan, the period and what is used against each limit.
- Settings shows a "Plan & usage" card: the plan, the period and what is used against each limit (from `GET /account/usage`), with plain-English copy for `QUOTA_EXCEEDED`.

### Fixed

- Host-proxy mode: MinIO's 80m memory cap made it thrash its page cache (327 GB read from disk in 3 hours, host IO stall, API 503). The default is now 192m with `GOMEMLIMIT=144MiB`, and `socialos-guard.sh` detects a thrashing container (memory at its cap plus fast `workingset_refault_file` growth), alerts naming it and restarts it once per cooldown instead of shedding services that cannot help (D-012). The README notes that scheduled GitHub workflows are best-effort and recommends an external uptime monitor.

## [0.2.0] - 2026-10-09

### Security

- The generated Claude Desktop config no longer runs `npx -y socialos-mcp`: that npm package does not exist, and whoever registered the name would have received users' API keys. Configs, the MCP connections page and the docs now use Claude Desktop connectors or the `mcp-remote` bridge pinned to an exact version, and a regression test rejects `socialos-mcp` and unpinned `npx` packages.

### Added

- Plan limits (D-014): opt-in limits for one `free` plan: connected accounts, scheduled or published posts per UTC month and media storage, all unlimited by default so updates never cap an existing install. A public instance sets them with `QUOTA_ACCOUNTS`, `QUOTA_POSTS_PER_MONTH` and `QUOTA_MEDIA_MB`; `-1` switches a limit off. Over a limit the action is refused with `403 QUOTA_EXCEEDED` and nothing changes. Drafts are free, a post counts once per month, reconnecting an account you already have is free. Counting is race-free, so parallel requests cannot overshoot.
- `GET /account/usage` (scope `analytics:read`) shows the plan, the period and what you have used against each limit.
- Dashboard motion: route transitions through the View Transitions API (with a CSS fade-and-rise fallback), a sidebar highlight that slides between items, dialog and toast enter/exit, staggered lists on first load, skeleton shimmer, button press feedback and card hover lift. All motion uses transform and opacity with tokens from `tokens.css` and is disabled under `prefers-reduced-motion`. Empty states gained an icon and page headers a clearer hierarchy.
- Public pages `/privacy` and `/terms` (no sign-in, linked from the sign-in and register forms and from the app sidebar and Settings), written for a self-hosted, single-operator instance and usable as the LinkedIn app's Privacy Policy URL. They are a template, not legal advice. The operator's name and contact come from the frontend container's `OPERATOR_NAME` and `OPERATOR_CONTACT`; without them the pages say so.
- Breaking: `POST /auth/register` requires `accept_terms: true` (otherwise `400 VALIDATION_ERROR` with `fields.accept_terms`). The register form has the matching checkbox. The accepted version and time are stored. Existing accounts are not blocked (D-016).
- A new landing page: full-viewport hero with an animated flow of posts from agent to approval to networks, a real recorded video of the demo (WebM and MP4, light and dark, poster only on phones and with reduced motion), a sticky "agent, approval, network" walkthrough, parallax screenshots, true-fact counters, a marquee of the networks that publish today and an honest list of those that do not. Cross-document View Transitions between the landing page and the docs. Docs keep their own calm layout. See D-017.
- Email verification, password reset and password change. Mailed links carry a single-use token in the URL fragment (48 h to verify, 30 min to reset). A reset signs out every session; a change keeps the current one. API keys and MCP connections are revoked only if you tick "Also revoke all API keys and MCP connections".
- When `MAIL_PROVIDER=smtp`, unverified accounts get `403 EMAIL_NOT_VERIFIED` on connecting networks, scheduling, publishing, editing scheduled posts and creating API keys. With the log provider nothing is restricted and the dashboard says mail is off. Existing accounts start unverified.
- New pages `/verify-email`, `/forgot-password` and `/reset-password`, an email banner, and a Password section in Settings.
- Database migration `00003` also prepares plans, quotas, data export and account deletion; it needs no manual step.
- Connect a network with a pasted credential: `POST /api/v1/social/accounts/token` and the provider capability fields `connect_fields`, `max_image_bytes` and `requires_title`. It needs the new critical API-key scope `social:connect`, which is never in a default set (D-009).
- "Connect with a token" on the Accounts page: Discord, Mastodon and Bluesky open a form built from the provider's `connect_fields` (secrets as password inputs with show/hide, never stored or put in a URL), with the how-to linked and plain-English errors per field. The demo and mock API answer the same fields and the token endpoint.
- Posts are checked against the stricter of the network limits and the account's own limits, and networks that need a title reject posts without one.
- SSRF-safe HTTP client for hosts users supply (D-010).
- Discord: connect a channel with its webhook URL and publish text and up to 10 images, or delete a post. The URL is stored encrypted and never returned; a timeout after sending goes to review instead of risking a duplicate. See [`docs/integrations/discord.md`](docs/integrations/discord.md).
- Connect fields have a `secret` flag (`connect_fields[].secret` in `GET /social/providers`) for credentials that are not of kind `secret`, such as a webhook URL; forms must render them as password inputs.
- Mastodon (and API-compatible Fediverse servers): connect with an instance URL and an access token, publish text and images (public), delete. Limits are read from the instance. A repeated publish after a timeout is safe because of the `Idempotency-Key`. See `docs/integrations/mastodon.md`.
- Bluesky: connect with a handle and an app password, publish text with link and hashtag facets and up to 4 images, delete, and resolve unknown outcomes through a deterministic record key. See `docs/integrations/bluesky.md`.
- Stub notes for Reddit, Medium and Hashnode; the X note now says the API is pay-per-use.
- Sharing a host safely (host-proxy mode): every SocialOS container runs in the systemd slice `socialos.slice` (1 CPU, 664 MB, no swap, 512 tasks for the whole stack), may not swap and has a process cap; drop-ins cap dockerd and containerd; a guard timer stops the SocialOS worker, then the worker, MCP and UI, only when the host is under pressure and SocialOS is a real contributor (anonymous memory, not page cache, or its own CPU or IO), and resumes them once the host is calm; it also alerts on disk, data budget and the other services' health. A Caddy pre-check keeps a broken SocialOS snippet from stopping the host's Caddy. Runbook: `deploy/host-proxy/apply-guardrails.md`.

- Approvals for dangerous actions (D-013): publishing now, retrying now, deleting a post, disconnecting an account, connecting with a pasted token, or scheduling less than 5 minutes ahead (`AGENT_MIN_SCHEDULE_LEAD`) with an API key or an MCP connection answers `428 APPROVAL_REQUIRED` and waits for you. Approve or deny in the dashboard under **Approvals** (`GET /approvals`, `POST /approvals/{id}/approve|deny`); the agent repeats the same call with `X-Approval-Id` and it works once. New settings `APPROVAL_TTL`, `APPROVAL_MAX_PENDING` (per key), `APPROVAL_RETENTION`, `AGENT_MIN_SCHEDULE_LEAD` (also gates edits of posts that run soon); new audit entries `approval.requested|approved|denied|used`.
- API keys have a `dangerous_policy`: `approve` (default, also for existing keys) or `trusted` (set when creating the key, skips approvals).

### Changed

- **Breaking for API-key integrations:** dangerous actions made with a key now need your approval (see above). The MCP tools `publish_post`, `delete_post` and `disconnect_account` no longer take `confirm`; they take an optional `approval_id`, and `schedule_post` and `update_post` take one too. An agent that only schedules at least 5 minutes ahead is not affected.
- Password hashing is bounded in memory: at most `PASSWORD_HASH_CONCURRENCY` (default 2) hashes run at once within `PASSWORD_HASH_MEMORY_MIB` (default 48) of argon2 memory, new hashes use argon2id m=19 MiB, t=2, p=1 (OWASP), and existing hashes are upgraded at the next successful login. Under a burst, logins and registrations may get `429 RATE_LIMITED` and should retry.
- The SSRF guard also blocks site-local `fec0::/10`, IPv4-compatible `::/96` and local-use NAT64 `64:ff9b:1::/48` (the whole range is blocked).
- Browser sessions now hold ten scopes (the new `social:connect`); the "Dangerous" list in the dashboard shows it.
- Host-proxy mode: memory caps rebalanced to fit the slice (backend and worker 160m with `GOMEMLIMIT=100MiB`, frontend 160m, postgres 112m, minio 80m, mcp 56m, redis 32m, migrate 64m). Automatic updates do not deploy while the guard has shed load.
- One shared table for post attempts, analytics, API usage, API keys and the audit log. On phones each row becomes a stacked card showing every column (the Error column is no longer hidden), `/developer` no longer scrolls sideways at 390 px, and wide tables can be scrolled with the keyboard.

### Fixed

- Large uploads no longer exhaust memory on a small host: media is streamed to object storage in 5 MiB parts (no temp files, no whole-file buffering), the size and type limits are enforced while streaming and the partial object is removed on failure, and at most `MEDIA_UPLOAD_CONCURRENCY` (2) uploads run at once, the rest waiting 5 s and then getting `429 RATE_LIMITED` with `Retry-After`. The web UI uploads directly to the API host, so videos are no longer cut at the Next.js proxy's 10 MB (D-015).
- Cancelling, unscheduling or publishing a post while the worker publishes it can no longer deadlock and fail with a 500: the API and the worker now lock rows in the same order (post, then target). A database deadlock or serialization failure that still happens is reported as a retryable `CONFLICT` (#38).
- Deploying or restarting the worker during a publish no longer sends that post to review: on SIGTERM the worker stops taking tasks, lets in-flight publishes finish (up to `WORKER_SHUTDOWN_TIMEOUT`, default 30s, at most 35s; the worker's `stop_grace_period` is now 45s instead of Docker's 10s), stops its background jobs and only then closes the database. The worker's health address is configurable as `WORKER_HTTP_ADDR` (#39).
- The Posts date filter now uses the timezone chosen in Settings instead of the browser's, so "From" and "To" cover whole days where you expect them.
- Colour contrast: unsupported-network badges and the dark-mode "Scheduled" badge now meet WCAG AA (axe `color-contrast` is clean in light and dark). The dark accent is slightly lighter, in the app and on the site.
- A failed load no longer looks like an empty list: Posts, post detail and the audit log show a titled error with a retry button, "Load more" failures keep the list on screen, and error messages are plain English (no raw codes, JSON or stack traces).

## [0.1.0] - 2026-10-09

First release: the MVP, ready to self-host on one server.

### Added

- Web dashboard to connect social accounts, write a post once, tailor it per network and publish it now or schedule it.
- REST API with browser sessions (CSRF-protected) and scoped, revocable API keys (`sk_live_…`, shown once, stored hashed).
- MCP server with 13 tools, filtered by the key's scope, so AI agents can draft, schedule and review posts (Streamable HTTP and stdio).
- Publishing to LinkedIn and Telegram; the other networks are shown honestly as "not available". Connecting a Telegram channel needs a one-time link code.
- Scheduler and worker with idempotent publishing, retries and a clear post state machine; media uploads to S3-compatible storage (bundled MinIO, R2 or S3).
- Production kit in [`deploy/`](deploy/README.md): Docker Compose with automatic HTTPS, `deploy.sh` (migrations, readiness wait, automatic rollback), backups and a runbook.
- Host-proxy mode for a small shared server: loopback-only ports, memory limits sized for about 1 GB, and an installer that adds one `import` line to the host's Caddy with validation, checks and automatic rollback.
- Pull-based updates: a systemd timer deploys the latest GitHub Release when its images exist. It never downgrades and can be switched off with `AUTOUPDATE=false`.
- Transactional mail through a queue, with SMTP and log-only adapters and templates for e-mail verification, password reset, password changed, account deleted and export ready.
- Audit log of every MCP tool call (tool, route, status, target ids, agent), with an "Agent actions" filter in the dashboard; the MCP server and the backend share a gateway secret so the real client IP is recorded.
- Setup guides for [LinkedIn](docs/integrations/linkedin.md) and [Telegram](docs/integrations/telegram.md), and a project site with an in-browser demo.

### Changed

- Dependencies refreshed: Go 1.26, pgx, minio-go, goose and other Go modules, frontend packages, Redis 8; CI actions moved to Node 24 on pinned runners.
- The bundled MinIO now uses a maintained, digest-pinned image because `minio/minio` was removed from Docker Hub.
- `deploy.sh` follows `COMPOSE_FILE`, cleans up only SocialOS images on shared hosts, and exits with 75 when it changed nothing.

### Fixed

- A scheduled post is never published before its `scheduled_at`; an early task is held briefly or handed back to the queue.
- The demo's sample data no longer breaks in the first minutes of a month.

### Security

- The client address is read from the right of `X-Forwarded-For` and only through trusted proxies (`TRUSTED_PROXIES`), so a forged header can no longer earn a fresh login or register rate-limit bucket.
- Rate limits group IPv6 clients by /64 and track a bounded number of keys.
- Post targets are locked and read per user, with tests that no other user can read, change or lock them.
- A Telegram channel can only be connected by proving ownership with a one-time link code.
- CI scans every change: govulncheck, npm audit, gitleaks over the history, Trivy on the images and CodeQL; the runtime images no longer ship npm.

[Unreleased]: https://github.com/XXX1694/steerpost/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/XXX1694/steerpost/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/XXX1694/SMSI/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/XXX1694/SMSI/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/XXX1694/SMSI/releases/tag/v0.1.0

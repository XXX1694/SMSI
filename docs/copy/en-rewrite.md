# English copy audit (code as of the `.worktrees/agents-md` base, 2026-10-09)

80 strings, grouped by surface. Paths are relative to the repo root; line numbers point to the current text. `{x}` is a
placeholder. "ICU" means the proposed text is an ICU `plural` message (see translation-process.md). Each rewrite follows
[style-guide.md](style-guide.md) and [glossary.csv](glossary.csv). Honesty fixes are checked against PLATFORMS.md and the
code they describe.

## Top 10

1. **#45: the MCP permission notice implies that SocialOS checks your consent.** It does not: the agent sets `confirm: true`
   itself. Say so.
2. **#70: the landing page says agents "can draft and schedule".** Scheduling is off by default, and the risky actions rely
   on a flag that the agent sets.
3. **#42: the media delete dialog warns that drafts "may lose the attachment".** In fact the backend refuses to delete a file
   that a post uses (`media/service.go:247`).
4. **#46: the "Create drafts" permission promises "upload media".** No MCP tool uploads media (`mcp/src/tools`).
5. **#14, #51-#54: every unavailable network says "Requires platform approval".** X needs a paid API, YouTube only allows
   private uploads, and LinkedIn pages need the Community Management API, not the Marketing Developer Platform.
6. **#33, #34: "Needs review" reads like the planned approval flow.** Rename it "Unconfirmed" and state the duplicate risk.
7. **#11: the dashboard empty state tells production users to connect "the mock provider".** The mock is refused in
   production.
8. **#17, #50, #29: raw codes and ids leak into sentences.** For example "Connection failed (access_denied)",
   "linkedin account @x" and "connection is expired".
9. **#38, #8: label collisions.** "Cancel" (the post action) sits next to "Cancel" (dismiss the dialog), and "Retry" both
   reloads data and republishes a post.
10. **#74-#76: MCP descriptions.** The risk prefix is duplicated ("[risk: sensitive; …] SENSITIVE: …"), the disconnect tool
    says "redo the OAuth flow" (Telegram uses a code), and the scope hint sends agents to a "developer portal" grant that
    does not exist (keys cannot gain scopes).

## Navigation and page headers

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 1 | frontend/src/components/developer/developer-nav.tsx:7 | API keys & logs | API keys & audit log | names real content |
| 2 | frontend/src/app/(app)/developer/layout.tsx:10 | API keys, MCP agents, usage and audit history. | API keys, MCP connections, usage and audit log. | one term |
| 3 | frontend/src/app/(app)/compose/page.tsx:9 | Write once, tailor per platform, publish or schedule. | Write once, adjust per network, then schedule or publish. | glossary term |

## Auth and onboarding

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 4 | frontend/src/components/auth-form.tsx:47 | Start composing and scheduling in a minute. | Next, you'll connect your first account. | honest expectation |
| 5 | frontend/src/components/auth-form.tsx:51 | This is a demo with a built-in account, so the form is already filled in. Just press Sign in. | Demo account: the form is filled in. Select Sign in. | shorter, no "just" |
| 6 | frontend/src/components/auth-form.tsx:85-87 | No account? Register | No account? Create one | one term |
| 7 | frontend/src/components/auth-form.tsx:27 | Password must be at least 8 characters. | Password is too short. Use at least 8 characters. | what + fix |

## Shared states and client errors

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 8 | frontend/src/components/states.tsx:27 | Retry | Try again | frees "Retry" |
| 9 | frontend/src/components/states.tsx:22 | Could not load this | Loading failed | clear subject |
| 10 | frontend/src/lib/api.ts:65 | You are not allowed to do that. | You do not have permission to do this. | glossary term |

## Dashboard

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 11 | frontend/src/components/dashboard-view.tsx:70 | Connect LinkedIn, Telegram or the mock provider to start publishing. | Connect a network to start publishing. | mock is dev-only |
| 12 | frontend/src/components/dashboard-view.tsx:66 | Go to accounts | Connect account | verb + object |
| 13 | frontend/src/components/dashboard-view.tsx:77 | No failures. Nice. | No failed posts. | no cheerleading |

## Accounts

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 14 | frontend/src/components/accounts-view.tsx:20 | Not supported yet. Requires platform approval. | Not available yet: {reason} (from `capabilities.notes`; see #51-#54) | accurate per network |
| 15 | frontend/src/components/accounts-view.tsx:108 | Some features (e.g. company pages) require platform approval. | (delete; `capabilities.notes` already says it) | duplicate, vague |
| 16 | frontend/src/components/accounts-view.tsx:144 | Connected {connected} successfully. | {network} connected. (show the label, not the raw id `linkedin`) | shorter, proper name |
| 17 | frontend/src/components/accounts-view.tsx:145 | Connection failed ({failed}). Please try again. | {network} was not connected: {reason}. Try again. (map the `error` code to a message) | no raw code |
| 18 | frontend/src/components/accounts-view.tsx:157 | {n} account(s) belong to providers no longer offered. | ICU: {count, plural, one {# account belongs} other {# accounts belong}} to a network that is no longer available. | ICU plural |
| 19 | frontend/src/components/accounts-view.tsx:162 | Scheduled posts targeting {name} will no longer publish. You can reconnect later. | Scheduled posts for {name} will fail unless you reconnect it before they are due. | precise consequence |
| 20 | frontend/src/lib/status.ts:31 | Expired (account badge) | Needs reconnecting. Also add a Reconnect button: `AccountItem` only offers Disconnect today. | action, not state |
| 21 | frontend/src/components/capability-badges.tsx:9 | Native scheduling (struck through when unsupported) | Scheduled by network | reads as "no scheduling" |
| 22 | frontend/src/components/accounts-view.tsx:88 | Mock · for testing | Test network | glossary term |

## Composer

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 23 | frontend/src/components/composer/composer-view.tsx:72, :79 | No accounts connected / Connect at least one account before composing a post. | No accounts yet / Connect an account to write your first post. | empty-state pattern |
| 24 | frontend/src/components/composer/composer-view.tsx:144 | Fix before continuing | ICU: Fix {count, plural, one {# problem} other {# problems}} first | says how many |
| 25 | frontend/src/components/composer/composer-view.tsx:170 | This posts immediately to {list} and cannot be undone from SocialOS. | This posts to {accounts} now. SocialOS cannot undo it. | shorter |
| 26 | frontend/src/components/composer/content-editor.tsx:27 | All platforms | All networks | glossary term |
| 27 | frontend/src/components/composer/content-editor.tsx:47 | Used for every selected account unless customised in its tab. | Goes to every account without custom text. | shorter, US spelling |
| 28 | frontend/src/components/composer/content-editor.tsx:65 | Empty: the universal content is used. | Empty, so the main text is used. | no jargon |
| 29 | frontend/src/lib/composer.ts:48 | {label}: connection is {status}. Reconnect it first. | {account} needs reconnecting. Reconnect it in Accounts. | no raw status |
| 30 | frontend/src/lib/composer.ts:61 | {label}: {n} characters over the {provider} limit of {max}. | ICU: {account}: {over, plural, one {# character} other {# characters}} over the {network} limit of {max, number}. | ICU plural |
| 31 | frontend/src/components/composer/schedule-fields.tsx:27-28 | Times are in {tz} · stored as {iso} UTC ({local} local). Change in Settings. | Time zone: {tz}. Change it in Settings. | drop internals |
| 32 | frontend/src/components/posts/post-detail.tsx:102 | Pick a date and time at least a minute in the future. | Choose a time at least 1 minute from now. (one key, shared with lib/composer.ts:82) | one message |

## Posts

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 33 | frontend/src/lib/status.ts:26 | Needs review | Unconfirmed | not an approval |
| 34 | frontend/src/components/posts/post-detail.tsx:38 | The outcome is unknown. Check the platform before retrying to avoid a duplicate. | SocialOS cannot confirm this was published. Check {network} before you retry, or it may post twice. | states the risk |
| 35 | frontend/src/components/posts/post-detail.tsx:42 | {n} attempt{n === 1 ? '' : 's'} | ICU: {count, plural, one {# attempt} other {# attempts}} | ICU plural |
| 36 | frontend/src/components/posts/post-detail.tsx:47 | View on platform | View on {network} | names destination |
| 37 | frontend/src/components/posts/post-detail.tsx:179 | Retry failed | Retry | reads as status |
| 38 | frontend/src/components/posts/post-detail.tsx:180 | Cancel | Cancel post. In its dialog the dismiss button becomes "Keep post". | two "Cancel"s |
| 39 | frontend/src/components/posts/post-detail.tsx:206 | It will not be published. Cancelled posts cannot be revived. | It will never publish, and this cannot be undone. To reuse the text, write a new post. | idiom, next step |
| 40 | frontend/src/components/posts/post-detail.tsx:186 | Targets | Accounts | user term |
| 41 | frontend/src/components/posts/posts-view.tsx:110-118 | No posts match these filters (no action) | No posts match these filters + [Clear filters] | next step |

## Media and settings

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 42 | frontend/src/components/media-view.tsx:100 | {file} will be removed from your library. Drafts using it may lose the attachment. | {file} will be removed from your library. Files used in a post cannot be deleted. | false today |
| 43 | frontend/src/components/media-library-dialog.tsx:50 | Attach {n} selected | ICU: {count, plural, one {Attach # file} other {Attach # files}} | ICU plural |
| 44 | frontend/src/components/settings-view.tsx:59 | Password changes are session-only and not yet available in this build. | Changing your password is not available yet. | plain words |

## Developer and MCP screens

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 45 | frontend/src/components/developer/mcp-view.tsx:101 | Dangerous tools still require the agent to pass an explicit confirm flag, but they can act on live accounts. | The agent must send a confirmation flag for these actions, but SocialOS cannot check that you agreed. Grant them only to agents you trust. | honest limit |
| 46 | frontend/src/lib/scopes.ts:62 | Agents can create and edit drafts and upload media. | Agents can create and edit drafts with media you uploaded. | no MCP upload |
| 47 | frontend/src/lib/scopes.ts:20 | Permanently remove posts. | Delete posts from SocialOS. Published copies stay. | soft delete |
| 48 | frontend/src/components/developer/mcp-view.tsx:115, :161 | No agents connected / Connected agents | No MCP connections yet / MCP connections | one term |
| 49 | frontend/src/components/developer/audit-view.tsx:117 | `actor_type.replace('_', ' ')` → "api key", "user" | Mapped labels: "API key", "Person" | no raw ids |

## Backend messages shown in the UI

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 50 | backend/internal/application/posts/validate.go:139 | %s account @%s needs to be reconnected | {network} account {name} needs reconnecting. Reconnect it in Accounts. | proper names |
| 51 | backend/internal/adapters/stubs/stubs.go:23, :34 | UNSUPPORTED in this release: requires a paid X API tier with write access. | Not available yet: X charges per post through its paid API. | per PLATFORMS |
| 52 | backend/internal/adapters/stubs/stubs.go:30 | requires Meta app review for instagram_content_publish and a Business/Creator account. | Needs an Instagram Business or Creator account. Posting for other people needs Meta review. | per PLATFORMS |
| 53 | backend/internal/adapters/stubs/stubs.go:33 | requires Google OAuth verification for youtube.upload scope. | Until Google verifies the app, uploads can only be private. | per PLATFORMS |
| 54 | backend/internal/adapters/linkedin/linkedin.go:95 | Company pages require Marketing Developer Platform approval. No native scheduling; SocialOS schedules. | Company pages need Community Management API approval. SocialOS does the scheduling. | per PLATFORMS |
| 55 | backend/internal/application/auth/service.go:96 | invalid email or password | Wrong email or password. | sentence case |
| 56 | backend/internal/infrastructure/postgres/users.go:37 | an account with this email already exists | An account with this email already exists. Sign in instead. | adds next step |
| 57 | backend/internal/transport/middleware/auth.go:107 | missing or invalid CSRF token | Your session expired. Reload the page. | user can act |
| 58 | backend/internal/application/media/service.go:247 | media is attached to a post | This file is used in a post. Remove it from the post first. | adds next step |
| 59 | backend/internal/application/posts/create.go:110 | post in status %s cannot be edited | Only drafts and scheduled posts can be edited. | states rule |
| 60 | backend/internal/application/posts/lifecycle.go:159 | post is %s; use retry | This post failed on some accounts. Retry it instead of publishing again. | no raw status |

## Emails

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 61 | backend/internal/adapters/mail/templates/verify_email.txt.tmpl:1 (and .html.tmpl:3) | Confirm your email address for SocialOS by opening this link: | Verify your email address for SocialOS: | one verb |
| 62 | backend/internal/adapters/mail/templates/reset_password.txt.tmpl:1 | Someone asked to reset the password of your SocialOS account. Open this link to choose a new one: | Someone asked to reset your SocialOS password. To choose a new one, open this link: | shorter |
| 63 | backend/internal/adapters/mail/templates/reset_password.txt.tmpl:5 | … If this was not you, ignore this message; your password stays the same. | … If you did not ask for this, ignore this email. Your password stays the same. | no semicolon |
| 64 | backend/internal/adapters/mail/templates/password_changed.txt.tmpl:1 | The password of your SocialOS account was just changed, and all other sessions were signed out. | Your SocialOS password was changed. Other sessions are signed out. | active, shorter |
| 65 | backend/internal/adapters/mail/templates/password_changed.txt.tmpl:3 | If this was not you, reset your password now and contact the operator of this server. | If this was not you, reset your password now: {ResetLink}. Then tell your server admin. | add the link |
| 66 | backend/internal/adapters/mail/render.go:33 | `ExpiresIn` is pre-formatted English ("48 hours") | Pass a duration, and format it per locale with plural rules. Subjects (:36-42) move to the catalog. | i18n blocker |

## Landing page

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 67 | site/src/pages/index.html:4 | One place to publish, for you and your AI agents. | AI agents draft and schedule your posts. You stay in control. | states the promise |
| 68 | site/src/pages/index.html:5 | SocialOS connects your social accounts, lets you write a post once, tailor it per platform and schedule it. The same abilities are available to AI agents through an MCP server, with scoped, revocable keys. | Write a post once, adjust it per network and schedule it. Claude Desktop, Claude Code or any MCP client can do the same, with keys you scope and can revoke. | shorter, concrete |
| 69 | site/src/pages/index.html:27-28 | Compose once, schedule per platform / … retries transient failures without ever posting twice. | Write once, adjust per network / … retries safe failures. If it cannot tell whether a post went out, it stops and asks you. | no absolute promise |
| 70 | site/src/pages/index.html:32 | … paste the config into your agent. It can draft and schedule; anything risky needs an explicit confirmation. | … paste the config into your agent. By default it can only read and draft. Scheduling, publishing and deleting stay off until you allow them. | true defaults |
| 71 | site/src/pages/index.html:62 | Two networks work today. The rest are registered but disabled, because each needs platform approval or a business account. | Two networks work today. The others need platform approval, a business account or a paid API, so they are off. | X is paid |
| 72 | site/src/pages/index.html:70 | … Company pages need Marketing Developer Platform approval from LinkedIn; video is not supported yet. | … Company pages need LinkedIn's Community Management API approval. Video is not supported yet. | per PLATFORMS |
| 73 | site/src/pages/index.html:94, :100-101 | An MCP server with a leash / Nine scopes. … need `confirm: true` on every call. | Agents work within limits you set / (drop the count, which goes stale once `social:connect` lands) … need `confirm: true` on every call. The agent sets that flag; server-side approval is planned. | idiom, honest limit |

## MCP tool descriptions (they stay English; see languages.md)

| # | File:line | Current | Proposed | Why |
|---|---|---|---|---|
| 74 | mcp/src/tools/write.ts:107, :121 | SENSITIVE: publishes the post … / SENSITIVE: deletes a post in SocialOS (soft delete). … | Publishes the post … / Deletes a post in SocialOS. Published copies stay on the networks. Requires `confirm: true` after the user approves. (server.ts:25 already prefixes `[risk: …]`) | duplicate risk label |
| 75 | mcp/src/tools/write.ts:135 | CRITICAL: disconnects a social account …; the user must redo the OAuth flow to reconnect, and pending scheduled posts for it will fail. … | Disconnects a social account and deletes its stored credentials. Its scheduled posts fail until the user reconnects it in the SocialOS UI. Requires `confirm: true` after explicit user approval. | not only OAuth |
| 76 | mcp/src/errors.ts:21 | … Ask the user to grant it in the SocialOS developer portal. | … Keys cannot gain scopes. Ask the user to create a new MCP connection with this permission under Developer → MCP connections. | real next step |
| 77 | mcp/src/tools/write.ts:37 | Previously uploaded media ids | Ids of files the user uploaded to the SocialOS media library. This server cannot upload files. | states limit |
| 78 | mcp/src/tools/write.ts:85 | … The post WILL go public at that time unless cancelled with cancel_scheduled_post. | … The post goes public at that time with no further confirmation, unless canceled with cancel_scheduled_post. Show the user the final text and accounts first. | human in control |
| 79 | mcp/src/server.ts:10-12 | … Prefer create_draft, then schedule_post. Never call publish_post, delete_post or disconnect_account without … | Add: "Before schedule_post, show the user the final text, accounts and time; scheduled posts publish without another check." | human in control |
| 80 | mcp/src/tools/write.ts:14 | … (e.g. shorter for X) … | … (for example, a shorter text for one network) … | X unsupported |

## Patterns to fix while extracting strings (not counted above)

- **Hand-made plurals**: post-detail.tsx:42, composer.ts:67, accounts-view.tsx:157 and media-library-dialog.tsx:50 → ICU
  `plural`.
- **Labels made from identifiers**: calendar view buttons (`'month' | 'week' | 'day'` with CSS `capitalize`,
  calendar-view.tsx:175-181), `actor_type.replace('_', ' ')` (audit-view.tsx:117), `metricLabel()` (analytics.ts:28) and
  `providerLabel()` capitalizing unknown ids (normalize.ts:83) → one catalog key per value.
- **Hardcoded locales**: `'en-GB'` (time.ts:88, calendar.ts:72, :84), `'en'` (time.ts:103) and `toLocaleString()` with no
  locale (analytics-view.tsx:62-63, usage-view.tsx:20, :37, capability-badges.tsx:25) → the formatter with the UI locale.
  The Monday week start is hardcoded in calendar.ts:31, :42.
- **Concatenated sentences**: account-chips.tsx:27 (`Connection is ${status}`), accounts-view.tsx:32 (`connected {date}`),
  post-detail.tsx:41-42, :172 and schedule-fields.tsx:27-28 → one message with placeholders.
- **British spellings** in UI: customised (content-editor.tsx:33, :47, :59), recognised (telegram-connect.tsx:266).
- **Limits that disagree**: the key name has `maxLength={60}` in the UI (api-keys-view.tsx:81) but a 100-character limit in
  the backend (developer/service.go:122). Pick one.

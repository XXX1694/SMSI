# SocialOS publishing platforms (checked 2026-10-09)

Legend: **[V]** means checked in the vendor's own docs today. **[U]** comes from secondary sources and has not been verified. Code paths are under `backend/internal/`.

## Summary

| Platform | Auth: what the user creates | Gate for own account | Cost | Post types | Hard limits | Token life | Tier |
|---|---|---|---|---|---|---|---|
| Discord | Webhook URL (channel → Integrations → Webhooks) | none | free | text, ≤10 embeds, files, threads (`thread_id`/`thread_name`), delete via `?wait=true` id [V] | 2000 chars [V]; ≤10 embeds [V]; ~5 req/2 s [U]; files: the reference states 20 MiB per file [V] but older docs said 8–10 MB, so we cap at 10 MiB and ≤10 files [U] | until the webhook is deleted | **live** ([setup](integrations/discord.md)) |
| Slack | Incoming-webhook URL (a Slack app with Incoming Webhooks) | none | free | text and Block Kit; no delete, no `ts` returned [V]; no files | ~1 msg/s per channel [V]; ~40k chars / 50 blocks [U] | until revoked | 1 |
| Mastodon (+ Pixelfed, GoToSocial [U]) | Personal token: Preferences → Development → New application (`write:statuses write:media read:accounts`) | none | free | text, media, replies, delete, `Idempotency-Key` kept 1 h [V] | per instance via `/api/v2/instance` (default 500 chars, 4 media) [U] | until revoked | 1 (live) |
| Bluesky (**live**) | Handle + app password | none | free | text with link and hashtag facets, ≤4 images, delete (replies not yet) | 300 graphemes + 3000 bytes [V]; images ≤2,000,000 bytes each [V]; `createSession` 30/5 min and 300/day [V]; 5000 write points/h and 35,000/day, create costs 3 [V] | session JWTs are short-lived, the app password lasts until revoked | 1 |
| Dev.to | API key (Settings → Extensions) | none | free | Markdown article with a required title and ≤4 tags; images by URL only; no delete (unpublish only) | ~10 creates/30 s [U] | until revoked | 1 |
| VK | Community access key with the `wall` right | none | free | wall text, photos | 10,000 calls/month for unverified partners from 2026-09-07 [V]; `wall.post` with a community key [U] | until revoked | 1 (verify live) |
| Misskey/Sharkey | Token from Settings → API (`write:notes`) | none | free | notes, files | per instance | until revoked | 1 |
| WordPress (self-hosted) / Ghost | Application password (Basic auth) / Admin key `id:secret` signed into a 5-min JWT | none | free if self-hosted | articles | — | until revoked | 1 (low priority) |
| Nostr | nsec key + relay list | none | free | kind-1 notes | per relay | the key can never be rotated | 1 (key-custody risk) |
| Telegram | done | — | free | text, image, video | — | — | done |
| Tumblr | OAuth 2.0 with client secret, no PKCE documented; app at tumblr.com/oauth/apps [V] | none | free | NPF text, photo, video | 250 posts/day, 250 images/day [V] | access ~42 min; refresh with `offline_access` [V] | 2 |
| Threads | OAuth 2.0, Meta app with the Threads use case | app-role users need no review [V]; anyone else needs App Review and a published app | free | text (500), image, video, carousel, replies | ~250 posts/24 h [U] | 1 h, then 60-day long-lived [U] | 2 (owner) / 3 |
| Instagram | OAuth (Instagram Login), Business or Creator account | Standard Access is enough for own or managed accounts; others need Advanced Access + Business Verification [V] | free | image, carousel, reels; media must be at a public URL | 100 posts/24 h (the reference says 50) [V, docs conflict] | 60 days, refreshable [U] | 2 (owner) / 3 |
| Facebook Pages | OAuth, `pages_manage_posts` | dev mode for app-role users [V general, U for this permission] | free | text, link, photo, video | — | Page token from a long-lived user token [U] | 2 (owner) / 3 |
| LinkedIn org | Community Management API | registered legal organisation + verification by a Page super admin [V] | free | — | — | — | 3 |
| X | OAuth 2.0 PKCE | paid | pay-per-use: $0.015/post, $0.20/post with a URL [V]; free tier closed to new developers on 2026-02-06 [U] | text, media | — | — | 3 (paid) |
| Reddit | OAuth | every new credential needs Responsible Builder approval (since 2025-11) [U] | free | — | — | — | 3 |
| YouTube | Google OAuth | unverified projects can only upload private videos [V]; refresh tokens last 7 days in Testing mode [U] | free | video | — | — | 3 |
| TikTok | OAuth 2.0 | audit required; unaudited clients post private only [V, 2026-08-04] | free | video, photo | — | 24 h [U] | 3 |
| Pinterest | OAuth 2.0 | Trial pins are visible only to the owner; Standard needs a video review [U] | free | pins | — | — | 3 |
| Hashnode | PAT | Pro plan required since May/June 2026 [U] | $5–7/mo | — | — | — | 3 (paid) |
| Max | Bot token; `platform-api2.max.ru`, `GET /me` [V] | verified business profile (ИП or legal entity) [U] | free | channel messages | — | until revoked | 3 |
| Medium | Integration token | no new tokens since 2025-01-01 [U] | — | — | — | — | Not feasible |
| WhatsApp Channels | none | no official Meta API; only unofficial QR gateways [U] | — | — | — | — | Not feasible |

Caveats:
- Bluesky tells new apps to use OAuth, but its docs still accept password auth for bots and CLIs [V], so plan a move to OAuth.
- Meta's app-modes page says content created in Development mode is visible only to role users. Check this on a test Page and a test Threads account.
- VK now allows only documented methods.
- Webhook URLs are bearer secrets: whoever holds one can post.

## Tiers

- **Tier 1 (ship now):** Discord, Mastodon, Bluesky, Slack, Dev.to, VK (pending a live check), Misskey, WordPress/Ghost. Nostr waits for a decision on how to hold the key.
- **Tier 2 (self-serve OAuth on the sslip.io HTTPS redirect):** Threads, Instagram and Facebook Pages for the owner as an app role; Tumblr; Bluesky OAuth.
- **Tier 3:** X (paid), Reddit, YouTube, TikTok, Pinterest, LinkedIn org, Hashnode (paid), Max.
- **Not feasible:** Medium (closed), WhatsApp Channels (no official API).

## Design: connect with a token

**Decision:** one generic port and one route, with each adapter declaring its own form.
- Rejected: an endpoint per provider, like `POST /social/telegram/connect`. That means N handlers and N UIs.
- Rejected: storing the fields in `metadata`. `accountDTO` returns metadata (`transport/http/dto.go:58`), so secrets would leak.

**Port** (`adapters/provider/provider.go`):
```go
const ConnectToken = "token"                 // beside :13-15
type ConnectField struct {
    Name, Label, Help, Placeholder string
    Kind     string                          // "text" | "secret" | "url"
    Required bool
}
// Capabilities (:19) gains:
//   ConnectFields []ConnectField `json:"connect_fields,omitempty"`
//   MaxImageBytes int64          `json:"max_image_bytes,omitempty"`
//   RequiresTitle bool           `json:"requires_title,omitempty"`
type TokenConnector interface {
    // Verify makes a live whoami call. Profile.Metadata holds non-secret data only;
    // secret is the adapter-encoded credential.
    Verify(ctx context.Context, fields map[string]string) (p Profile, secret string, err error)
}
```
- `PublishRequest` (:140) gains `Title`. `buildRequest` (`application/scheduler/execute.go:64`) fills it from `post.Title` (`domain/post/post.go:21`).
- The registry gains `TokenConnector(name)`, built like `OAuth()` (`registry.go:57-70`).

**Use case** in the new `application/accounts/token.go`: `ConnectWithToken(ctx, a actor.Actor, providerName string, fields map[string]string) (*socialaccount.Account, error)`.
1. `a.Require(apikey.SocialConnect)`. This is a new **critical** scope in `domain/apikey/scopes.go:16-43` and is never in the default set.
2. Check the fields against `ConnectFields`: unknown keys are rejected, required keys must be present, each value is ≤2 KB, and `url` fields must be https.
3. Call `Verify` with a 10 s timeout. A new `connectFailure` maps KindAuth or KindPermanent to `400 VALIDATION_ERROR` ("X rejected these credentials"). Everything else goes through `providerFailure` (`oauth.go:131`).
4. Call `accountFromProfile` (`oauth.go:116`), then `connectAccount(..., &provider.Token{AccessToken: secret})` (`service.go:127`). This reuses the same upsert, which also sets status, so reconnecting turns `expired` back into `active`. It also reuses the AES-GCM vault (`vault.go:24`) and the audit entry.

**HTTP:** `POST /api/v1/social/accounts/token {provider, fields}` returns `201 accountDTO`. Mount it in `mountAuthenticated` (`transport/http/router.go:116`) behind the `link:` limiter, as Telegram does (:120).

**Worker:** `execute.go:33` loads credentials for `ConnectOAuth` **or** `ConnectToken`. Static tokens have no `ExpiresAt`, so `NeedsRefresh` is false (`domain/socialaccount/account.go:46`) and they pass straight through.

**What is secret and what is metadata:**
- Secret: the whole webhook URL, the app password, the API key.
- Metadata: host, handle, DID, channel id, and `limits` (instance `max_characters` and media count).
- `provider_account_id` per adapter: Discord webhook id, Mastodon `host:id`, Bluesky DID, Dev.to user id, VK `-group_id`, Slack `sha256(url)[:16]`.

**Honest capabilities:**
- `CheckContent` (`application/posts/validate.go:149`) applies the stricter of the provider limits and `metadata.limits`, and also checks `RequiresTitle` and `MaxImageBytes`.
- It counts runes, which is stricter than Bluesky's grapheme count, so it errs on the safe side.
- The composer reads `/social/providers`. MCP sees the limits through `list_social_accounts` metadata. REST stays the place where limits are enforced.

**Error mapping** (`*provider.Error`, `adapters/provider/errors.go:46`):

| Signal | Kind | Effect |
|---|---|---|
| 401; Discord 404 code 10015; Slack 403 `invalid_token` / 404 `no_service`; Bluesky `AuthenticationRequired`; VK error 5 | Auth | target fails with `SOCIAL_ACCOUNT_EXPIRED`, account becomes `expired` (`execute.go:135`, `finalize.go:99`); the UI shows Reconnect, which repeats the same POST |
| 429 with Retry-After or `ratelimit-reset`; VK error 6 | Retryable | backoff honours the provider's delay |
| timeout after the request was sent | Unknown | `needs_review`, except Mastodon (`Idempotency-Key`, so `SafeToRetryAfterUnknown`) and Bluesky (`Lookuper` with a deterministic rkey + `getRecord`) |

**SSRF guard** in the new `adapters/safehttp`, for adapters whose host the user supplies:
- The dialer `Control` rejects loopback, private, link-local, CGNAT and multicast addresses.
- https only, redirects only to the same host, response capped at 1 MB.
- Fixed-host adapters allow-list their host instead (`discord.com`, `hooks.slack.com`).
- Every adapter takes `Config.HTTPClient` so tests can inject a client.

**Tests:**
- Each adapter gets a fake server, following `linkedin/linkedin_test.go:94-101`: an `httptest.Server` with whoami, publish, upload and delete handlers, and `APIBaseURL = srv.URL`.
- Cases: profile metadata holds no secret; 401 → Auth; 429 → Retryable with its delay; 500 → Retryable; a handler slower than the client timeout → Unknown; request headers and body asserted; no secret appears in `err.Error()`.
- A shared `adapters/providertest.Contract(t, p)`: a `ConnectToken` provider has at least one secret field, and its caps are consistent.
- `e2e/security_test.go`: an API key without `social:connect` gets 403; user B gets 404 on A's account; no response ever contains a secret.

## Rollout

Each live test publishes something publicly, so it needs the owner's "yes" and a private target (CLAUDE.md "Do not").

- **PR0a (backend):** port, registry, use case, scope, route, safehttp, worker change, `CheckContent`, providertest, a mock token provider. Add the next free `docs/DECISIONS.md` entries: token connect with the critical `social:connect` scope, and the SSRF guard. Update the stub notes in `adapters/stubs/stubs.go:29-37` (X is now pay-per-use; add Reddit, Medium, Hashnode). Acceptance: connect → publish with the mock works end to end; the scope tests pass.
- **PR0b (frontend):** `components/accounts-view.tsx:90` branches on `capabilities.connect_method`. A new `token-connect.tsx` renders `connect_fields` (secret fields use `type=password` and `autocomplete=off`). The composer shows a title field when `requires_title` is set. Attach Playwright screenshots.
- **PR1 Discord (live):** fixed host (`discord.com`, `discordapp.com`); `?wait=true` returns the id needed for delete. `webhook_url` is a `Secret` url field. Text and images only (no embeds, threads or video yet). A timeout after send is Unknown, not retried: Discord has no idempotency key and no message listing for a webhook, so no `Lookuper` is possible and `needs_review` is the honest outcome.
- **PR2 Mastodon (live):** the first user-supplied host, so the SSRF guard runs live; also idempotency. Setup: [integrations/mastodon.md](integrations/mastodon.md). The same PR closed three guard gaps: site-local `fec0::/10`, IPv4-compatible `::/96` and local-use NAT64 `64:ff9b:1::/48` (the whole range is blocked).
- **PR3 Bluesky (done, `adapters/bluesky`):** app-password connect, link and hashtag facets (UTF-8 byte offsets), blob upload, deterministic TID rkey, `Lookuper` via `getRecord`, delete via `deleteRecord`. Sessions are cached in memory per account (refresh before create, back off on 429). The core counts runes, never fewer than graphemes, so it is stricter than Bluesky; the adapter re-checks graphemes and the 3000-byte cap. Alt text is sent empty (the core media model has no alt field), mentions are not linked. App passwords are discouraged for new apps [V]; OAuth stays in Tier 2. A custom PDS URL goes through `safehttp`; `bsky.social` is a fixed host. The TID layout and the duplicate-rkey behaviour of `createRecord` are [U]. Setup: [integrations/bluesky.md](integrations/bluesky.md).
- **PR4 Slack:** text only, `CanDelete=false`. To validate, POST `{}` and expect `400 no_text` [U].
- **PR5 Dev.to:** title; set `published:false` for the test.
- **PR6 VK:** first a live `wall.post` with a community key. If VK rejects it, VK moves to Tier 2 (VK ID OAuth 2.1).
- **Then:** Misskey and WordPress. Nostr after a decision on key custody (a NIP-46 bunker is preferred).
- **Tier 2 next:** Threads, Instagram, Facebook Pages, Tumblr, Bluesky OAuth. Each uses `/social/{p}/connect` with the callback at `https://api.194-238-43-194.sslip.io/api/v1/social/{p}/callback`.

**Acceptance for every adapter PR:**
1. The fake-server tests are green.
2. Connect → the account is `active` and no secret appears in the response.
3. Publish → the `external_url` opens.
4. Revoke the token at the provider, then publish → `SOCIAL_ACCOUNT_EXPIRED`.
5. Reconnect → `active`.

**Testing with an API key** that has `social:read social:connect posts:write posts:publish`:
```bash
API="https://api.194-238-43-194.sslip.io/api/v1"; KEY="sk_live_..."
H=(-H "Authorization: Bearer $KEY" -H "Content-Type: application/json")
curl -s "${H[@]}" "$API/social/providers" | jq '.items[]|select(.capabilities.connect_method=="token")|{name,f:.capabilities.connect_fields}'
ACC=$(curl -s "${H[@]}" -X POST "$API/social/accounts/token" \
  -d '{"provider":"discord","fields":{"webhook_url":"https://discord.com/api/webhooks/ID/TOKEN"}}' | jq -r .id)
POST=$(curl -s "${H[@]}" -X POST "$API/posts" -d "{\"content\":\"SocialOS test\",\"social_account_ids\":[\"$ACC\"]}" | jq -r .id)
curl -s "${H[@]}" -X POST "$API/posts/$POST/publish"; curl -s "${H[@]}" "$API/posts/$POST/status"
```
Fields for the other providers:
- mastodon: `{instance_url, access_token}`
- bluesky: `{handle, app_password}`
- slack: `{webhook_url}`
- devto: `{api_key}`
- vk: `{group_id, access_token}`

## Risks

- **Bearer secrets:** webhook URLs and app passwords grant full posting rights to whoever holds them. Mitigation: the vault, redaction tests, and the critical scope.
- **Bluesky:** `createSession` is capped at 300 logins/day, and the app-password path is discouraged for new apps.
- **SSRF** through user-supplied instance URLs.
- **VK:** monthly API quota.
- **Meta:** content created in Development mode may be visible only to role users.
- **Unverified figures:** everything marked [U]. Read limits live where the API allows it (Mastodon instance, Instagram `content_publishing_limit`).

## Sources (accessed 2026-10-09)

[Discord webhooks](https://docs.discord.com/developers/resources/webhook) · [Slack incoming webhooks](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks) · [Mastodon statuses](https://docs.joinmastodon.org/methods/statuses/) · [Bluesky rate limits](https://docs.bsky.app/docs/advanced-guides/rate-limits) · [atproto SDK auth](https://atproto.com/guides/sdk-auth) · [VK overview](https://dev.vk.ru/en/api/overview) · [Max API](https://dev.max.ru/docs-api) · [Tumblr API](https://www.tumblr.com/docs/en/api/v2) · [Threads get started](https://developers.facebook.com/docs/threads/get-started) · [IG publishing](https://developers.facebook.com/docs/instagram-platform/content-publishing) · [Meta app modes](https://developers.facebook.com/documentation/development/build-and-test/app-modes) · [LinkedIn CM migration](https://learn.microsoft.com/en-us/linkedin/marketing/community-management/community-management-api-migration-guide) · [X pricing](https://docs.x.com/x-api/getting-started/pricing) · [YouTube videos.insert](https://developers.google.com/youtube/v3/docs/videos/insert) · [TikTok direct post](https://developers.tiktok.com/doc/content-posting-api-reference-direct-post) · [Ghost Admin API](https://docs.ghost.org/admin-api) · [WP app passwords](https://developer.wordpress.org/advanced-administration/security/application-passwords/) · [Misskey tokens](https://misskey-hub.net/en/docs/for-developers/api/token/) · [Reddit RBP thread](https://red.applefritter.com/r/redditdev/s/ae5A05NG39) · [Hashnode paywall](https://dev.to/thefreetier/hashnodes-free-blog-platform-quietly-paywalled-its-entire-api-4f92) · [Medium tokens](https://docs.stackone.com/connectors/medium/guides/link-account/integration-token) · [Pinterest trial](https://community.pinterest.biz/t/stuck-in-trial-mode-pins-not-showing-publicly/44410) · [X pricing guide](https://www.wearefounders.uk/the-x-api-price-hike-a-blow-to-indie-hackers/) · [Bluesky limits mirror](https://adaptlypost.com/blog/bluesky-api-rate-limit) · [WhatsApp gateway](https://whapi.cloud/blog/whatsapp-channel-api-automation)

## Decision on connecting from an API key

Connecting a network from an API key is allowed only with the critical `social:connect` scope, which is never in a key's
default set. Browser sessions can always connect. Once server-side approvals land (goal 5), a key-initiated connect needs the
owner's approval like other dangerous actions.

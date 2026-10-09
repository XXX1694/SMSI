# Social providers and OAuth setup

What each network supports today, how to add a provider, and how to set up LinkedIn and Telegram. Step-by-step operator
guides: [LinkedIn](linkedin.md) and [Telegram](telegram.md). Which networks come next, and what each one requires, is in
[PLATFORMS](../PLATFORMS.md).

## Capabilities, honestly reported

`GET /api/v1/social/providers` returns each provider's `capabilities`, which the UI uses to enable features and enforce limits: `can_publish_text`, `can_publish_image`, `can_publish_video`, `can_schedule`, `can_delete`, `can_analytics`, `max_text_length`, `max_media_count`, `requires_approval` and `notes`.

| Provider | Status | Notes |
|---|---|---|
| LinkedIn | Real | OAuth 2.0 + OpenID `userinfo`, `/rest/posts`. Text and up to 20 images; video not in the MVP. Personal profiles via "Share on LinkedIn". **Company pages need Marketing Developer Platform approval** (`requires_approval`) |
| Telegram | Real | Bot API, not OAuth. One bot serves all users, so a chat is connected by posting a one-time code in it (proof of control), never by typing its name. Text, image, video (up to 10 media), delete |
| Mock | Dev/test only | Deterministic; content containing `#mock-fail`, `#mock-retry`, `#mock-auth` or `#mock-unknown` simulates failures |
| Instagram, Facebook, TikTok, YouTube, X, Threads, Pinterest | Not available | Registered stubs returning `PROVIDER_NOT_AVAILABLE`. All need app review or business accounts, so they are not pretended to work |

## Adding a new social provider

The design uses small, optional interfaces (`backend/internal/adapters/provider/provider.go`) instead of one large `SocialProvider` interface. A provider implements only what the network really supports, and capabilities say the rest. The brief's methods map as follows:

| Brief | Interface method |
|---|---|
| Connect | `OAuth.AuthorizeURL` + `Exchange`, or `ChatVerifier.VerifyChat` for non-OAuth networks |
| RefreshToken | `OAuth.Refresh` |
| GetAccount | `OAuth.Profile` |
| PublishPost / CreatePost | `Publisher.Publish` (receives `IdempotencyKey`) |
| DeletePost | `Publisher.Delete` |
| GetPost | `Lookuper.Lookup` (used for crash recovery) |
| UpdatePost, GetAnalytics | Not in the MVP: no MVP network supports editing, and analytics needs approvals. Reported via `can_analytics=false` |

```go
// backend/internal/adapters/twitter/twitter.go
package twitter

type Provider struct{ cfg Config; http *http.Client }

func New(cfg Config) *Provider { … }
func (p *Provider) Name() string                        { return "x" }
func (p *Provider) DisplayName() string                 { return "X" }
func (p *Provider) Configured() bool                    { return p.cfg.ClientID != "" }
func (p *Provider) Supported() bool                     { return true }
func (p *Provider) Capabilities() provider.Capabilities { return provider.Capabilities{CanPublishText: true, MaxTextLength: 280, …} }
func (p *Provider) AuthorizeURL(a provider.AuthorizeParams) string { … }   // OAuth
func (p *Provider) Exchange(ctx context.Context, code, verifier, redirect string) (provider.Token, error) { … }
func (p *Provider) Profile(ctx context.Context, token string) (provider.Profile, error) { … }
func (p *Provider) Refresh(ctx context.Context, refresh string) (provider.Token, error) { … }
func (p *Provider) Publish(ctx context.Context, r provider.PublishRequest) (provider.PublishResult, error) { … }
func (p *Provider) Delete(ctx context.Context, r provider.DeleteRequest) error { … }
```

Then:

1. Register the adapter in `internal/app/app.go` with `reg.Register(twitter.New(…))`. It replaces the stub of the same name.
2. Add its config variables.
3. Write adapter tests against an `httptest` fake, using `adapters/linkedin/*_test.go` as the template.
4. Classify errors with `provider.Error{Kind: KindRetryable|KindPermanent|KindAuth|KindRateLimited}`, so the worker knows whether to retry.

No core business logic changes. Networks that connect with a token instead of OAuth follow the "connect with a token" design in [PLATFORMS](../PLATFORMS.md).

## LinkedIn

1. Create an app at <https://www.linkedin.com/developers/apps>.
2. Add the products **Sign In with LinkedIn using OpenID Connect** and **Share on LinkedIn**.
3. Add the redirect URL `${API_PUBLIC_URL}/api/v1/social/linkedin/callback`.
4. Put `LINKEDIN_CLIENT_ID` and `LINKEDIN_CLIENT_SECRET` in `.env`.

The requested scopes are `openid profile email w_member_social`. Posting to company pages additionally needs Marketing Developer Platform access. The full walkthrough, with troubleshooting, is in the [LinkedIn guide](linkedin.md).

## Telegram

SocialOS runs **one bot for the whole deployment**. If connecting only needed the channel's `@username`, any user could attach somebody else's channel, because the bot is an admin of all of them. So a chat is linked only when the user proves they control it with a one-time code.

Setup (operator):

1. Create a bot with @BotFather and set `TELEGRAM_BOT_TOKEN`.
2. Choose how updates arrive with `TELEGRAM_UPDATES_MODE`:
   - `polling` (default): nothing else to do. The worker long-polls Telegram (`getUpdates`), so it works behind NAT and on localhost. Only one worker polls at a time (Redis lease), so you can run several. Do not run another consumer of the same bot token.
   - `webhook`: set `TELEGRAM_WEBHOOK_SECRET` (for example `openssl rand -hex 24`), make `API_PUBLIC_URL` an https address Telegram can reach and run `make -C backend telegram-set-webhook` (add `URL=https://…/api/v1/webhooks/telegram` to override). `make -C backend telegram-webhook-info` shows what Telegram has registered and `telegram-delete-webhook` removes it. Switching back to `polling` requires deleting the webhook first (Telegram refuses `getUpdates` while one is set; the worker logs a hint).

Connecting (user, on `/accounts`):

1. Click **Connect channel**. SocialOS shows a code such as `SOS-7KQ2M9XA`, valid for 15 minutes and usable once.
2. Add the bot to your channel or group as an administrator with the **Post messages** right.
3. Post the code in that chat as a normal message. The page detects it within a couple of seconds, shows the connected chat and the bot deletes your message.

Notes:

- In a group or supergroup only an administrator's code counts (anyone can post there). In a channel only admins can post anyway.
- A wrong, expired or reused code is ignored silently: the bot never replies in the chat.
- If the bot lacks the right to post, the code is kept: fix the permission and post it again before it expires.
- A bot that is an administrator receives every message of a group, so Telegram's privacy mode does not stop the bot from seeing the code. If a group still shows nothing, check that the bot really is an administrator.

The full walkthrough is in the [Telegram guide](telegram.md).

## The OAuth flow

After `GET /api/v1/social/{provider}/connect`:

1. Generate a random state; store its hash, the user, a PKCE verifier and a 10-minute TTL.
2. Redirect to the provider's consent page.
3. On `/callback`, validate the state: it must be unused, unexpired and owned by the session user.
4. Exchange the code and fetch the profile.
5. Upsert the account by the provider account ID (never by token) and encrypt its tokens.
6. Write an audit entry and redirect to `/accounts?connected=…`.

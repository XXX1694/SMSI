# Owner setup: accounts, platforms and a first test

For the owner of the production instance. It lists, network by network, what you set up once on the platform side, what
every user then does in the Steerpost UI, and how to check the whole chain without posting anything public.

Production runs without a domain of its own, on sslip.io names with valid Let's Encrypt certificates:

| What | URL |
|---|---|
| Web app | <https://app.194-238-43-194.sslip.io> |
| REST API (OAuth callbacks, Telegram webhook) | <https://api.194-238-43-194.sslip.io> (all routes under `/api/v1`) |
| MCP server | <https://mcp.194-238-43-194.sslip.io/mcp> |

Status of each network, from the code (`backend/internal/adapters/`) and [PLATFORMS](PLATFORMS.md), checked 2026-10-09:

| Network | Status | Platform-level setup by you | Per user |
|---|---|---|---|
| Telegram | live | done (bot @ABZAL_SOCIALMEDIABOT, webhook set) | add the bot to a channel, post a code |
| LinkedIn (personal profile) | live | one LinkedIn developer app, two env vars | OAuth consent |
| Discord | live | none | paste a channel webhook URL |
| Mastodon | live | none | create a token on their own server |
| Bluesky | live | none | handle + app password |
| LinkedIn Company Pages, Instagram, Facebook, Threads, TikTok, YouTube, X, Pinterest, Reddit, Medium, Hashnode | stubs: shown, never pretend to work (`adapters/stubs/stubs.go`) | see section 3 | none |

## How to change server settings (secrets never go into chat)

Platform-level secrets (for example the LinkedIn client secret) live only in `/opt/socialos/.env` on the server. Never paste
them into chat, issues, git or screenshots. Put them there yourself:

```bash
ssh irbisa
sudo nano /opt/socialos/.env           # edit or add the lines, save (Ctrl+O, Enter, Ctrl+X)
cd /opt/socialos && sudo docker compose up -d --wait
```

`docker compose up -d` recreates only the containers whose configuration changed (`COMPOSE_FILE` in `.env` already selects
the right compose files). Values are plain: no quotes, no spaces, no trailing comments. `deploy.sh` has no restart option
(it takes `<tag>`, `--rollback`, `--status`, `--no-migrate`). If you prefer a restart with a health check and an automatic
rollback, redeploy the running tag instead:

```bash
cd /opt/socialos && sudo ./deploy.sh "$(sudo cat .deploy/current_tag)" --no-migrate
```

## 1. Create your Steerpost account

1. Open <https://app.194-238-43-194.sslip.io/register>, enter your name, email and a password, submit.
2. Email verification: production runs `MAIL_PROVIDER=log`, so **no email is sent** (the worker only logs "mail not sent",
   with a masked address and, outside development, without the link). The banner asking you to verify can be ignored.
3. With `MAIL_PROVIDER=log` verification is **not enforced**: the API enforces it only when `MAIL_PROVIDER=smtp`
   (`requireVerification` in `backend/internal/app/app.go`). You can create posts, publish and create API keys right away.
4. Consequence: password reset mail is not delivered either. Store your password in a password manager. Once real mail is
   switched on (section 4), every existing account must verify once before it can publish.

## 2. Platforms

### 2.1 Telegram (live)

**Platform-level setup (once, by the owner).** Already done: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_UPDATES_MODE=webhook` and
`TELEGRAM_WEBHOOK_SECRET` are in the server `.env`, and the webhook points at
`https://api.194-238-43-194.sslip.io/api/v1/webhooks/telegram`. Nothing to do. To check it:
`cd /opt/socialos && sudo docker compose run --rm telegram webhook-info` (expect that URL and no `last error`).

**Per-account connect (in the Steerpost UI).**

1. Create a **private** Telegram channel for testing (Telegram > New Channel > Private), or use an existing channel or group.
2. In Steerpost open **Accounts** and click **Connect channel** (Telegram). A code such as `SOS-7KQ2M9XA` appears. It is valid
   15 minutes and works once.
3. In Telegram: channel > **Administrators > Add admin** > `@ABZAL_SOCIALMEDIABOT`, keep the **Post messages** right on.
4. Post the code in the channel as a normal message. Within a few seconds the page shows the connected chat and the bot deletes
   your message.
5. Nothing happens? The bot never replies to wrong, expired or reused codes, or to codes from non-admins in a group. Check that
   the bot is an admin with Post messages, then start a new code.

Typing a channel name never connects it: the code proves you control the chat. Full guide: [integrations/telegram.md](integrations/telegram.md).

### 2.2 LinkedIn, personal profile (live)

What Steerpost uses, from the code: env vars `LINKEDIN_CLIENT_ID` and `LINKEDIN_CLIENT_SECRET` (optional
`LINKEDIN_API_VERSION`, default `202606`; `LINKEDIN_USE_PKCE`, default `true`) in `backend/internal/config/config.go`, passed
through by `deploy/docker-compose.prod.yml`; scopes `openid profile email w_member_social`
(`backend/internal/adapters/linkedin/linkedin.go`); callback route `GET /api/v1/social/{provider}/callback`
(`backend/internal/transport/http/router.go`) on `API_PUBLIC_URL`, which production sets to `https://api.${DOMAIN}`.

The exact redirect URL for production is therefore:

```
https://api.194-238-43-194.sslip.io/api/v1/social/linkedin/callback
```

**Platform-level setup (once, by the owner).**

1. **A LinkedIn Page.** Every developer app must be associated with a LinkedIn Page. If you have none, create one at
   <https://www.linkedin.com/company/setup/new/> (a small placeholder Page is fine for the self-serve products). Steerpost posts
   to each connecting member's own profile, not to this Page. The association cannot be changed later.
2. **A privacy policy URL.** The create-app form has a *Privacy policy URL* field. Steerpost has no privacy page yet, so publish
   a short one on any public https page you control (for example a GitHub Pages page of this repo or a public Gist) and use that
   URL. An sslip.io URL would also be syntactically fine, but there is no such page on the app today.
3. Open <https://www.linkedin.com/developers/apps/new> and fill in:
   - **App name:** `Steerpost` (no "LinkedIn" or "In" in the name or logo);
   - **LinkedIn Page:** the Page from step 1;
   - **Privacy policy URL:** the URL from step 2;
   - **App logo:** a square image of at least 100 x 100 px, uploaded from your computer;
   - tick the legal agreement, click **Create app**.
4. **Verify the app:** *Settings* tab > **Verify** > **Generate URL**. A super admin of the Page opens that URL and confirms. If
   that is you, it is instant.
5. **Products** tab: click **Request access** on
   - **Sign In with LinkedIn using OpenID Connect** (grants `openid profile email`), and
   - **Share on LinkedIn** (grants `w_member_social`).
   Both are self-serve: accept the terms and they are granted at once, with **no app review**.
6. **Auth** tab > *OAuth 2.0 settings* > **Authorized redirect URLs for your app** > add exactly
   `https://api.194-238-43-194.sslip.io/api/v1/social/linkedin/callback`. Note the `api.` host (not `app.`), no trailing slash.
   Check that *OAuth 2.0 scopes* now lists `openid`, `profile`, `email`, `w_member_social`.
7. On the same *Auth* tab copy the **Client ID** and the **Primary Client Secret**.
8. Put them on the server (see "How to change server settings"):

   ```
   LINKEDIN_CLIENT_ID=<Client ID>
   LINKEDIN_CLIENT_SECRET=<Primary Client Secret>
   ```

   then `cd /opt/socialos && sudo docker compose up -d --wait`.

**Will the sslip.io address be accepted?** LinkedIn's documented rules for redirect URLs are: HTTPS, absolute, query parameters
ignored, no `#`. It documents no domain ownership check and no ban on wildcard-DNS hosts, and the certificate is a valid
Let's Encrypt one, so the sslip.io callback meets every published rule. This has not been tried in the portal yet: if the portal
rejects the URL, a real domain is the fix (point `DOMAIN` at it and update the redirect URL).

**What works without review:** sign-in and posting text and up to 20 images to the connecting member's own profile. Video is not
supported by the adapter. Tokens last 60 days and self-serve apps get no refresh token, so every member reconnects about every
60 days (the account shows **Reconnect**).

**Per-account connect (in the Steerpost UI).**

1. **Accounts > Connect > LinkedIn.** You are sent to linkedin.com; sign in and click **Allow**.
2. You land back on `/accounts` with the profile connected. Connect while logged in to the app in the same browser: the callback
   on `api.` needs your session cookie (`COOKIE_DOMAIN` defaults to `194-238-43-194.sslip.io`, valid for both `app.` and `api.`).

Errors and fixes: [integrations/linkedin.md](integrations/linkedin.md#troubleshooting). Company Pages: section 3.

### 2.3 Discord (live)

**Platform-level setup (once, by the owner).** None. No developer app, no env var.

**Per-account connect (in the Steerpost UI).**

1. For a private test, create your own Discord server (**+** > *Create My Own*), or use a private channel.
2. Channel > **Edit Channel > Integrations > Webhooks > New Webhook** (needs *Manage Webhooks*). Set the name and avatar posts
   should show, click **Copy Webhook URL**.
3. In Steerpost: **Accounts > Connect > Discord**, paste the URL into **Webhook URL**.
4. Only `https://discord.com/...` and `https://discordapp.com/...` URLs are accepted. The URL is a password: anyone who holds it
   can post. If it leaks, delete the webhook in Discord and reconnect with a new one.

Limits: 2000 characters, up to 10 images of 10 MB, delete supported. Guide: [integrations/discord.md](integrations/discord.md).

### 2.4 Mastodon (live)

**Platform-level setup (once, by the owner).** None. Each user creates a token on their own Mastodon server.

**Per-account connect (in the Steerpost UI).**

1. On your Mastodon server (for example <https://mastodon.social>): **Preferences > Development > New application**.
2. **Application name:** `Steerpost`. Leave *Redirect URI* at its default (`urn:ietf:wg:oauth:2.0:oob`; Steerpost does not use it).
3. **Scopes:** untick everything, then tick exactly `write:statuses`, `write:media`, `read:accounts` (the scopes in
   `backend/internal/adapters/mastodon/mastodon.go`). Click **Submit**.
4. Open the application and copy **Your access token**.
5. In Steerpost: **Accounts > Connect > Mastodon**, fill **Instance URL** (`https://mastodon.social`, no path) and **Access token**.

Posts are public on Mastodon; for a test use a throwaway account. Revoke: delete the application in Preferences > Development.
Guide: [integrations/mastodon.md](integrations/mastodon.md).

### 2.5 Bluesky (live)

**Platform-level setup (once, by the owner).** None.

**Per-account connect (in the Steerpost UI).**

1. In Bluesky: **Settings > Privacy and security > App passwords > Add App Password**, name it `Steerpost`, copy the
   `xxxx-xxxx-xxxx-xxxx` password. Never use your main password.
2. In Steerpost: **Accounts > Connect > Bluesky**, enter **Handle** (`name.bsky.social`) and **App password**. Leave
   **Server (optional)** empty unless you run your own PDS.

Limits: 300 characters, up to 4 images of 2 MB, delete supported, 300 logins per day per account. Bluesky has no private
posts: test on a throwaway account. Guide: [integrations/bluesky.md](integrations/bluesky.md).

## 3. Networks not supported yet (stubs)

They appear in the UI as unavailable and every operation fails with `PROVIDER_NOT_AVAILABLE`. What blocks each one:

| Network | Blocker |
|---|---|
| LinkedIn Company Pages | Community Management API is vetted: a registered legal organisation, business email, a new app dedicated to it, Page super-admin verification, Development then Standard tier with a screencast. Not self-serve. |
| X (Twitter) | Paid: pay-per-use (about $0.015 per post, $0.20 with a link); the free tier is closed to new developers. Needs a decision to spend money. |
| Instagram | Meta app + Instagram Business/Creator account; posting for anyone but app-role users needs App Review, Advanced Access and Business Verification. Live mode needs a privacy policy URL; media must be at a public URL. |
| Facebook Pages | Meta app review for `pages_manage_posts` plus Business Verification for other users' Pages; development mode covers app-role users only. |
| Threads | Meta app with the Threads use case: app-role testers work in development mode, everyone else needs App Review and a published app. |
| TikTok | Content Posting API audit; unaudited clients can only post privately. |
| YouTube | Google OAuth verification for the upload scope; unverified projects can only upload private videos and Testing-mode refresh tokens expire after 7 days. |
| Pinterest | Trial access shows pins only to the owner; Standard access needs a review with a video. |
| Reddit, Medium, Hashnode | Reddit: new credentials need Responsible Builder approval. Medium: no new integration tokens. Hashnode: API needs a paid plan. |

For the Meta networks the sslip.io HTTPS callback is fine for app-role testing in development mode
([PLATFORMS](PLATFORMS.md), Tier 2); Business Verification and a public launch realistically need a real domain and website.
None of these adapters exists yet, so nothing in this section can be set up today.

## 4. Optional owner accounts

| Service | Create | Env vars in `/opt/socialos/.env` | Blocker |
|---|---|---|---|
| Resend (real email) | Account at <https://resend.com>, add and verify a sending domain (DNS records), create an API key | `MAIL_PROVIDER=smtp`, `SMTP_HOST=smtp.resend.com`, `SMTP_PORT=465`, `SMTP_TLS=implicit` (or `587` + `starttls`), `SMTP_USERNAME=resend`, `SMTP_PASSWORD=<API key>`, `MAIL_FROM=Steerpost <no-reply@your-domain>` | **Needs a domain you own.** sslip.io names cannot be verified as a sending domain. Turning it on also enforces email verification for every account. |
| Sentry | Project at <https://sentry.io>, copy the DSN | none today | The code has no Sentry integration and reads no `SENTRY_DSN` (checked in `backend/internal/config`, `.env.prod.example`). Creating a project now is harmless but has no effect until it is wired in. |
| Offsite backups (S3, Backblaze B2 or Cloudflare R2) | A private bucket with a lifecycle rule that expires old objects; an access key that may only write to it. On **your own computer**: `age-keygen -o key.txt` and keep `key.txt` off the server | `BACKUP_S3_URL=s3://<bucket>/socialos`, `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY`, `BACKUP_S3_ENDPOINT` (empty for AWS; B2: `https://s3.<region>.backblazeb2.com`; R2: `https://<account>.r2.cloudflarestorage.com`), `BACKUP_AGE_RECIPIENT=age1...` (public key only) | none; without `key.txt` the offsite copies cannot be read |

Local backups need nothing (daily, 14 days, `/var/backups/socialos`). See [deploy/README.md](../deploy/README.md), section 9.

## 5. Test checklist

Use private targets only: the private Telegram channel and a Discord server of your own. Nothing is public unless you choose a
public target.

### 5.1 In the UI

- [ ] Log in at <https://app.194-238-43-194.sslip.io/login>.
- [ ] **Accounts:** connect the private Telegram channel (2.1) and a private Discord channel (2.3). Both show **active**.
- [ ] Optional: connect LinkedIn (2.2), Mastodon or Bluesky. Do not publish to them unless you want a public post.
- [ ] **Compose:** write "Steerpost test", pick the Telegram account, **Publish now**. The message appears in the channel and the
      post shows **published** with a link.
- [ ] **Posts:** delete that post; the message disappears from the channel.
- [ ] Schedule a post 10 minutes ahead to the Discord account; it arrives on time (**Calendar** shows it until then).
- [ ] **Developer:** create an API key (next section).

### 5.2 With an API key: the approvals flow

Every dangerous action by an API key (publish now, retry, delete, disconnect, connect with a token, schedule less than 5 minutes
ahead) is answered with `428 APPROVAL_REQUIRED` until you approve it in the browser (D-013). Only a browser session can approve.

1. **Developer > API keys > Create**: name `owner-test`, scopes `social:read posts:read posts:write posts:publish
   posts:delete`. Leave **Trusted key: do not ask me before dangerous actions** unticked (the default, `dangerous_policy: approve`). Copy the `sk_live_...` key once. Keep it out of chat.
2. In a terminal on your computer (`jq` needed):

   ```bash
   API="https://api.194-238-43-194.sslip.io/api/v1"
   read -rs "KEY?API key: "; echo          # zsh; in bash: read -rsp "API key: " KEY; echo
   H=(-H "Authorization: Bearer $KEY" -H "Content-Type: application/json")

   curl -s "${H[@]}" "$API/social/accounts" | jq '.items[]|{id,provider,display_name,status}'
   ACC="<id of the private Telegram or Discord account>"

   # Create a draft: not dangerous, no approval needed
   POST=$(curl -s "${H[@]}" -X POST "$API/posts" \
     -d "{\"content\":\"Steerpost API test\",\"social_account_ids\":[\"$ACC\"]}" | jq -r .id)

   # Publish now: dangerous -> 428
   curl -s -w '\nHTTP %{http_code}\n' "${H[@]}" -X POST "$API/posts/$POST/publish"
   # {"error":{"code":"APPROVAL_REQUIRED",...,"fields":{"approval_id":"...","approve_url":".../approvals",...}}}  HTTP 428
   ```

3. Open <https://app.194-238-43-194.sslip.io/approvals>, check the summary, click **Approve** (within 10 minutes).
4. Repeat the identical call with the approval id:

   ```bash
   APPROVAL="<approval_id from the 428>"
   curl -s -w '\nHTTP %{http_code}\n' "${H[@]}" -H "X-Approval-Id: $APPROVAL" -X POST "$API/posts/$POST/publish"
   curl -s "${H[@]}" "$API/posts/$POST/status" | jq
   ```

   The message appears in the private channel. Reusing the same `X-Approval-Id` for another call gets a fresh 428: one
   approval covers one action on one target.
5. Deny path: call publish on a second draft, click **Deny** on `/approvals`, retry with its id: `403` "the owner denied this
   action".
6. Clean up: delete the test posts (each delete is another 428 + approve, or delete them in the UI), then revoke the key in
   **Developer**.

### 5.3 Platform checks that post nothing

```bash
curl -s https://api.194-238-43-194.sslip.io/api/v1/ready      # backend and its dependencies
curl -s "${H[@]}" "$API/social/providers" | jq '.items[]|{name,configured,supported}'   # linkedin configured=true once the env vars are in
```

## Sources (read 2026-10-09)

- LinkedIn 3-legged OAuth, redirect URL rules, 60-day tokens: <https://learn.microsoft.com/en-us/linkedin/shared/authentication/authorization-code-flow>
- LinkedIn quick start (Page, app creation, Products tab): <https://learn.microsoft.com/en-us/linkedin/marketing/quick-start>
- Create an app for a LinkedIn Page (create-app form fields): <https://www.linkedin.com/help/learning/answer/a1667239>
- Share on LinkedIn (`w_member_social`): <https://learn.microsoft.com/en-us/linkedin/consumer/integrations/self-serve/share-on-linkedin>
- Sign In with LinkedIn using OpenID Connect: <https://learn.microsoft.com/en-us/linkedin/consumer/integrations/self-serve/sign-in-with-linkedin-v2>
- Programmatic refresh tokens (partners only): <https://learn.microsoft.com/en-us/linkedin/shared/authentication/programmatic-refresh-tokens>
- Community Management app review: <https://learn.microsoft.com/en-us/linkedin/marketing/community-management-app-review>
- Discord webhooks: <https://docs.discord.com/developers/resources/webhook>
- Mastodon statuses API: <https://docs.joinmastodon.org/methods/statuses/>
- Bluesky rate limits: <https://docs.bsky.app/docs/advanced-guides/rate-limits>
- Resend SMTP: <https://resend.com/docs/send-with-smtp>
- Platform blockers (X, Meta, TikTok, YouTube, Pinterest, Reddit, Medium, Hashnode) with their own sources: [PLATFORMS](PLATFORMS.md#sources-accessed-2026-10-09)

# LinkedIn setup guide

For the owner of a Steerpost instance. You create one LinkedIn developer app per deployment; every user then connects their own profile
through it. Steps match the LinkedIn Developer Portal as documented in October 2026.

## What you get

| Capability | Needs | Steerpost status |
|---|---|---|
| Sign in + read profile (name, picture, email) | Product **Sign In with LinkedIn using OpenID Connect** (self-serve) | works |
| Post text and images to a **personal profile** | Product **Share on LinkedIn** (self-serve) | works |
| Post to a **Company Page** | Community Management API, vetted by LinkedIn | shown as **requires LinkedIn approval**; Steerpost does not request `w_organization_social` |

## Steps

1. **Have a LinkedIn Company Page.** A developer app must be associated with a Page. If you have none, create a Page first
   (a placeholder Page is accepted for the self-serve products). The Page is only the app's owner of record: Steerpost posts to the
   connecting member's profile, not to this Page.
2. Open <https://www.linkedin.com/developers/apps> and click **Create app**. Give it a name without "LinkedIn" or "In" logos, select
   the Page, add a privacy policy URL and a logo, accept the API terms.
3. **Verify the app.** *Settings* tab > *Verify* > **Generate URL**. A **super admin of the Company Page** opens that URL and
   confirms the association. If you are the admin, this is instant; otherwise send the link to the admin. The app shows "Verified" afterwards.
4. **Products** tab: request **Sign In with LinkedIn using OpenID Connect** and **Share on LinkedIn**. Both are self-serve
   (tick, accept terms; no review). They grant `openid profile email` and `w_member_social`.
5. **Auth** tab > *Authorized redirect URLs for your app* > add exactly:

   ```
   ${API_PUBLIC_URL}/api/v1/social/linkedin/callback
   # production example: https://api.example.com/api/v1/social/linkedin/callback
   ```

   It must be absolute https, with no query and no `#`. It is the **api** host, not the app host (`app.<domain>`).
   LinkedIn's docs ask for an HTTPS callback; for local development use a tunnel or check what the portal accepts today.
6. On the *Auth* tab copy the **Client ID** and **Primary Client Secret**.
7. Put them in the server's `.env` (see below), then `docker compose up -d` so the containers pick them up.
8. Connect an account in the UI (**Accounts > Connect > LinkedIn**), approve the consent screen, publish a test post.

## What Steerpost requests (from the code)

- Scopes: `openid profile email w_member_social` (default in `backend/internal/adapters/linkedin/linkedin.go`, `New`).
- Authorization URL `https://www.linkedin.com/oauth/v2/authorization`, token URL `https://www.linkedin.com/oauth/v2/accessToken`,
  userinfo `https://api.linkedin.com/v2/userinfo`, posts `/rest/posts` (header `LinkedIn-Version`, default `202606`,
  override with `LINKEDIN_API_VERSION`).
- PKCE (S256) is on by default (`LINKEDIN_USE_PKCE=true`); `state` is always sent.
- Callback route: `GET /social/{provider}/callback` under `/api/v1` (`backend/internal/transport/http/router.go`), so
  `${API_PUBLIC_URL}/api/v1/social/linkedin/callback`.
- Limits enforced by the adapter: text up to 3000 characters, up to 20 images, no video.
- If you change the scopes, every user must reconnect (LinkedIn invalidates earlier tokens when the scope set changes).

## Company Pages (requires LinkedIn approval)

Posting as an organization needs the `w_organization_social` permission from the **Community Management API**, which is a vetted
product, not self-serve. LinkedIn requires:

- a registered legal organization and a commercial use case;
- a verified business email (personal addresses fail), legal name, address, website and privacy policy;
- the app verified by a super admin of the organization's Page;
- a **new app dedicated to it**: the Development-tier request is greyed out for apps that already hold other products, and a rejected app cannot re-apply (create a new one);
- Development tier first (low rate limits), then Standard tier with a screencast of every use case.

Steerpost marks LinkedIn as `requires_approval` and states in the capability notes that company pages are not available yet. Do not
promise Company Page posting to users until this review is done and the adapter requests the extra scope.

## Token lifetime and reconnecting

- Access tokens last **60 days** (`expires_in`, 5184000 s). There is no way to extend them without the member.
- **Programmatic refresh tokens (365 days) are only for approved Marketing Developer Platform partners.** A self-serve app gets
  no `refresh_token`.
- Behaviour in Steerpost: the worker refreshes shortly before expiry only if a refresh token exists
  (`application/scheduler/execute.go`, `freshToken`). Otherwise the adapter returns `NO_REFRESH_TOKEN`/`TOKEN_EXPIRED`, the
  account is marked **expired**, the post target fails with "account must be reconnected", and the user clicks **Reconnect**.
- The UI should therefore tell users honestly: "LinkedIn asks you to reconnect about every 60 days". Reconnecting while still logged
  in to linkedin.com before expiry skips the consent screen.

## Where the secrets go

`LINKEDIN_CLIENT_ID` and `LINKEDIN_CLIENT_SECRET` live **only in the server's `.env`** (mode 600, backed up in a password manager).
Never put them in git, GitHub variables/secrets of this repo, issues, screenshots or chat. The deploy workflow does not need them.
If the secret leaks, use *Auth* > **Generate a new Client Secret** and update `.env`. Stored user tokens are encrypted with
`ENCRYPTION_KEY`; losing that key invalidates them.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| "The redirect_uri does not match the registered value" / 401 `Redirect_uri doesn't match` | The URL in the Auth tab differs from `${API_PUBLIC_URL}/api/v1/social/linkedin/callback` (scheme, host, trailing slash, `app.` vs `api.`). Fix `API_PUBLIC_URL` or the portal entry. |
| `unauthorized_scope_error` / "Invalid scope" | A requested scope is not granted to the app: a product is missing or still pending. Check *Auth* tab > *OAuth 2.0 scopes*; add the two products from step 4. |
| `?error=UNAUTHENTICATED` after consent | The browser returned to `api.<domain>` without the session cookie. `COOKIE_DOMAIN` (default `$DOMAIN`) must stay set so the cookie is valid on both `app.` and `api.` (deploy/README.md, section 7). |
| Provider "not configured" | `LINKEDIN_CLIENT_ID` or `_SECRET` empty in the running container; run `docker compose up -d` after editing `.env`. |
| `user_cancelled_authorize` | The member pressed Cancel. Not an error. |
| Posting works for a few weeks, then "must be reconnected" | Normal 60-day expiry, see above. |
| 403 on a Company Page URN | Expected: organization posting is not enabled. |

## Sources (read October 2026)

- OAuth 3-legged flow, redirect rules, token lifetime, errors (page updated 2026-05-15): <https://learn.microsoft.com/en-us/linkedin/shared/authentication/authorization-code-flow>
- Programmatic refresh tokens, MDP partners only (updated 2025-10-08): <https://learn.microsoft.com/en-us/linkedin/shared/authentication/programmatic-refresh-tokens>
- Share on LinkedIn, `w_member_social` (updated 2023-12-14): <https://learn.microsoft.com/en-us/linkedin/consumer/integrations/self-serve/share-on-linkedin>
- Sign In with LinkedIn using OpenID Connect (updated 2024-08-08): <https://learn.microsoft.com/en-us/linkedin/consumer/integrations/self-serve/sign-in-with-linkedin-v2>
- Community Management overview and FAQ (updated 2026-05-15): <https://learn.microsoft.com/en-us/linkedin/marketing/community-management/community-management-overview>
- Community Management app review requirements (updated 2026-02-11): <https://learn.microsoft.com/en-us/linkedin/marketing/community-management-app-review>
- Quick start, Page and app creation (updated 2026-08-17): <https://learn.microsoft.com/en-us/linkedin/marketing/quick-start>
- Associate an app with a Page: <https://www.linkedin.com/help/linkedin/answer/a548360/associate-an-app-with-a-linkedin-page>
- Community walkthroughs used only to cross-check the portal clicks (Settings > Verify, Products tab): <https://dev.to/broke2builtai/how-to-post-to-linkedin-from-a-script-the-15-minute-oauth-walkthrough-2026-58fp>, <https://agoras.readthedocs.io/en/latest/credentials/linkedin.html>

# Mastodon setup guide

For users of a Steerpost instance. Nothing to configure on the Steerpost server: each user connects their own account on their
own Mastodon server with a token. Works with Mastodon and servers that implement the same API (GoToSocial, Pixelfed and
similar; only Mastodon itself is tested).

## Connect

1. On your server open **Preferences > Development > New application**. Name it "Steerpost".
2. Tick exactly these scopes: `write:statuses`, `write:media`, `read:accounts`. Untick the rest.
3. Save, open the application and copy **Your access token**.
4. In Steerpost open **Accounts > Mastodon**, enter the instance URL (for example `https://mastodon.social`) and the token.
   Using the API: `POST /api/v1/social/accounts/token` with `{"provider":"mastodon","fields":{"instance_url":"...","access_token":"..."}}`.
   An API key needs the critical `social:connect` scope.

Steerpost checks the token with `GET /api/v1/accounts/verify_credentials`, reads the limits from `GET /api/v2/instance`
(`/api/v1/instance` on older servers) and stores them next to the account. The token is encrypted at rest and never shown again.
The account id is `host:id`, so the same person on two servers is two accounts.

## What it does

| | |
|---|---|
| Text | up to the instance limit (`max_characters`, 500 by default) |
| Images | up to the instance limit (4 by default), each up to `image_size_limit` (16 MiB by default). Large images are processed asynchronously by the server; Steerpost waits up to 60 s |
| Video | not supported |
| Visibility | public |
| Delete | supported (`DELETE /api/v1/statuses/:id`); an already deleted post counts as deleted |
| Link to the post | the `url` the server returns |

Limits can only be stricter than the defaults: an instance that allows more than 500 characters is still capped at 500 by Steerpost.

## Safety

- The instance URL must be `https://host[:port]` with no path, query or credentials.
- The server address is user-supplied, so every request goes through the SSRF-safe client (D-010): loopback, private,
  link-local (including the cloud metadata address), CGNAT, site-local and other reserved addresses are refused, and redirects
  only stay on the same host. **A server on a private network cannot be connected.**
- Retries: the post is sent with an `Idempotency-Key` derived from the post target, the same on every attempt. Mastodon keeps
  the key for about an hour and returns the first post for a repeat, so a timeout does not create a duplicate if the retry
  comes within that hour.

## Errors

| Server answer | Steerpost | What to do |
|---|---|---|
| 401, or 403 (missing scope) | account becomes `expired` | create a new token with the three scopes and reconnect |
| 422 | post fails, message from the server | fix the post (length, media type) |
| 429 | retried after `X-RateLimit-Reset` | none |
| 5xx, connection refused | retried with backoff | none |
| timeout while posting | retried safely (idempotency key) | none |

## Revoke

Preferences > Development > your application > delete it (or **Revoke** in Authorized apps). The next publish ends in
`SOCIAL_ACCOUNT_EXPIRED` and the account shows "Reconnect".

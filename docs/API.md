# REST API

Everything the web app and the MCP server do goes through this API. This page is the overview; the full contract (request
bodies, scopes, the database and the scheduler) is in [ARCHITECTURE](ARCHITECTURE.md).

## Errors

All endpoints are under `/api/v1`. Errors look like this:

```json
{"error":{"code":"SOCIAL_ACCOUNT_EXPIRED","message":"LinkedIn authorization has expired","request_id":"…"}}
```

## Endpoints

| Area | Endpoints |
|---|---|
| Auth | `POST /auth/register` (needs `accept_terms: true`) · `POST /auth/login` · `POST /auth/logout` · `GET /me` (user, scopes, csrf_token) · `POST /auth/verify-email` · `POST /auth/verify-email/resend` · `POST /auth/password/forgot` · `POST /auth/password/reset` · `POST /auth/password/change` |
| Social | `GET /social/providers` · `GET /social/accounts` · `GET /social/accounts/{id}` · `GET /social/{provider}/connect` · `GET /social/{provider}/callback` · `POST /social/telegram/connect` (no body, returns a link code) · `GET /social/telegram/connect/{id}` (link status) · `DELETE /social/accounts/{id}` |
| Posts | `POST /posts` · `GET /posts?status=&from=&to=&cursor=` · `GET /posts/{id}` · `PATCH /posts/{id}` · `DELETE /posts/{id}` · `POST /posts/{id}/publish` · `/schedule` · `/unschedule` · `/cancel` · `/retry` · `GET /posts/{id}/status` |
| Media | `POST /media` (multipart; images ≤ 10 MB, video ≤ 100 MB; MIME sniffed) · `GET /media` · `GET /media/{id}` · `DELETE /media/{id}` |
| Insights | `GET /dashboard/summary` · `GET /analytics` · `GET /audit-logs` |
| Developer | `GET/POST /developer/api-keys` · `DELETE /developer/api-keys/{id}` · `GET/POST /developer/mcp-connections` · `DELETE /developer/mcp-connections/{id}` · `GET /developer/usage` |
| Ops | `GET /health` · `GET /ready` · `GET /metrics` |
| Webhook | `POST /webhooks/telegram` (only when `TELEGRAM_UPDATES_MODE=webhook`; authenticated by the `X-Telegram-Bot-Api-Secret-Token` header, not by session or key) |

## Post lifecycle

Posts move `draft → scheduled → publishing → published | partially_published | failed`, and `draft|scheduled → cancelled`. Invalid transitions return `409 INVALID_STATE_TRANSITION`. Every attempt is recorded in `publication_attempts` with its number, times, status, error and response metadata.

## Idempotency

Each target is published at most once:

1. The worker locks the target and skips it if it is already published.
2. It writes the attempt row and passes `IdempotencyKey = target.id` to the adapter.
3. If an attempt was left `started` by a crash, the target becomes `needs_review` instead of being re-posted blindly. That prefers a visible missed post over a duplicate.

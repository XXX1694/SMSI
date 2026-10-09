# Telegram setup guide

For the owner of a Steerpost instance. Steerpost uses **one bot per deployment**, shared by all users. Telegram has no OAuth for
bots, so a user proves they control a channel or group with a one-time code (below).

## Steps

1. **Use your existing bot** from @BotFather (or create one with `/newbot`). Copy its token. Keep privacy defaults: the bot only
   needs to see the code message, which channel posts and admin messages always deliver.
2. Put the token in the server's `.env` as `TELEGRAM_BOT_TOKEN`. Only there: never in git, GitHub secrets, issues or chat. If it
   leaks, `/revoke` it in @BotFather and update `.env`. An invalid token shows up as `BOT_UNAUTHORIZED` ("Telegram bot token is invalid").
3. **Check who else uses this bot before you touch webhooks** (next section). Telegram allows exactly one update consumer per bot.
4. Choose the update mode with `TELEGRAM_UPDATES_MODE` (see "Webhook or polling") and `docker compose up -d`.
5. In the UI, open **Accounts > Connect channel** as a user and follow the link-code flow below to verify the whole chain.

## Do not steal updates from another consumer

`setWebhook` replaces whatever webhook the bot has, and `getUpdates` (polling) fails while a webhook is set. If another service
(an older script, n8n, another product) already consumes this bot, switching it over silently breaks that service.
**The `set-webhook` command does not check this for you.** Always run first:

```bash
docker compose run --rm telegram webhook-info
```

- `no webhook is set`: nobody uses a webhook. Another process may still poll `getUpdates`; if you cannot rule that out, use a new bot.
- `url: https://something-else/...`: another consumer owns the bot. Use a separate bot for Steerpost, or migrate that consumer first.
- `url:` equal to `https://api.<your-domain>/api/v1/webhooks/telegram`: already yours; `pending updates` and `last error` tell you
  whether Telegram can reach you.

## Webhook or polling

| Mode | Use | How |
|---|---|---|
| `webhook` | **Production** (`TELEGRAM_UPDATES_MODE=webhook`) | Needs `TELEGRAM_WEBHOOK_SECRET` (1-256 chars of `A-Za-z0-9_-`, 16+ in production, e.g. `openssl rand -hex 24`). Telegram posts to `POST /api/v1/webhooks/telegram` on the **api** host and echoes the secret in `X-Telegram-Bot-Api-Secret-Token`; a missing or wrong secret gets 401. |
| `polling` (default) | Local dev, behind NAT | The worker long-polls `getUpdates`; one worker at a time holds a Redis lease, so several can run. Nothing to register. |

Webhook commands (production stack, from the deploy folder; see also deploy/README.md section 6):

```bash
docker compose run --rm telegram webhook-info                 # url, pending updates, last error
docker compose run --rm telegram set-webhook                  # registers https://api.<domain>/api/v1/webhooks/telegram + secret
docker compose run --rm telegram set-webhook -drop-pending    # same, and discards queued old updates
docker compose run --rm telegram delete-webhook               # required before switching back to polling
```

Telegram delivers only to https on port 443/8443 with a valid certificate. Re-run `set-webhook` after changing the domain or the
secret (after `docker compose up -d`). The webhook asks only for `channel_post`, `message` and `my_chat_member` updates
(`AllowedUpdates` in `adapters/telegram/updates.go`).

## How the ownership proof works

Code: `backend/internal/application/accounts/chatlink.go`, `backend/internal/domain/linkcode`, `backend/internal/adapters/telegram`.

1. The user clicks **Connect channel**. Steerpost creates a code like `SOS-7KQ2M9XA`, shown once. Only its hash is stored. It is valid
   **15 minutes** (`linkcode.TTL`), usable once, and a user holds at most **3** active codes (starting a fourth retires the oldest).
2. The user adds the bot as **administrator with the "Post messages" right** (channel: *Administrators > Add admin*; group: same).
3. The user posts the code in that chat as a normal message.
4. The update reaches `HandleChatUpdate` (webhook or polling). A channel post is trusted (only admins can post). In a group the sender
   must be the creator or an administrator (`getChatMember`), anonymous admins included; other senders are ignored.
5. Steerpost re-checks the bot's rights with `getMe`, `getChat` and `getChatMember` (`VerifyChat`): only channels and groups,
   never private chats, and the bot must be allowed to post. Then the account is stored for the code's owner and the bot **deletes the
   code message** (best effort).
6. Wrong, expired or reused codes and non-admin senders get no reply, so nothing leaks. They are only visible in debug logs
   (the code itself is never logged). Typing a channel name never connects anything.

## Limits the adapter enforces

Source: `adapters/telegram/telegram.go` (constants and `Validate`), `upload.go`, `client.go`.

| Limit | Value | Error code |
|---|---|---|
| Text message | 4096 characters | `TEXT_TOO_LONG` |
| Caption (post with media) | 1024 characters | `CAPTION_TOO_LONG` |
| Media per post (album) | 10 items | `TOO_MANY_MEDIA` |
| Photo upload | 10 MB | `MEDIA_TOO_LARGE` |
| Video upload | 50 MB (Bot API upload cap) | `MEDIA_TOO_LARGE` |
| Media types | photo (`sendPhoto`), video (`sendVideo`), GIF (`sendAnimation`), 2-10 items as album (`sendMediaGroup`, caption on the first item) | |
| Delete | supported | |

Rate limits: Steerpost has no own throttle for Telegram. On HTTP 429 the adapter reads `retry_after` and classifies the failure as
retryable, so the worker retries with backoff (max 5). Telegram's own guidance is about 1 message per second per chat and 20 per
minute per group; schedule bursts accordingly. `403` ("bot was kicked / lost rights") is an auth failure: the user must re-add the
bot and reconnect.

## Troubleshooting

| Symptom | Fix |
|---|---|
| Code posted, nothing happens (webhook) | `webhook-info`: wrong `url`, `last error`, growing `pending updates`; check `TELEGRAM_WEBHOOK_SECRET` equals the one used at `set-webhook`, and that `TELEGRAM_UPDATES_MODE=webhook` is set. |
| Worker logs a hint about a webhook while polling | A webhook is still set: `delete-webhook`. |
| Another service stopped receiving updates | You replaced its webhook. Restore it, or give Steerpost its own bot. |
| Code expired | Click **Connect channel** again for a new one. |
| Group works, channel does not (or the reverse) | The bot must be an admin with "Post messages" in that chat. |

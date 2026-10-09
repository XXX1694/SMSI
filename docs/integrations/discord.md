# Discord setup guide

For users of SocialOS. Discord needs no developer app and no server-side configuration: each user connects a channel with an
**incoming webhook URL**. SocialOS posts into that channel as the webhook.

## Connect a channel

1. In Discord, open the channel you want to post in: **Edit Channel > Integrations > Webhooks > New Webhook**
   (you need the *Manage Webhooks* permission in that channel).
2. Give the webhook the name and avatar your posts should appear under, then **Copy Webhook URL**.
3. In SocialOS open **Accounts**, choose Discord and paste the URL. Through the API:
   `POST /api/v1/social/accounts/token {"provider":"discord","fields":{"webhook_url":"https://discord.com/api/webhooks/<id>/<token>"}}`
   (needs the critical `social:connect` scope).

## The webhook URL is a password

Anyone who holds the URL can post in that channel, with no further login. SocialOS therefore:

- accepts only `https://discord.com/...` and `https://discordapp.com/...` URLs and never contacts another host;
- stores the URL encrypted, and never returns it, logs it or writes it to the audit trail;
- keeps only the webhook id, channel id, server (guild) id and name as account data.

Do not paste the URL into chat, issues or git. If it leaks, delete the webhook in Discord (**Integrations > Webhooks**); the
account then shows as expired, and you reconnect with a new webhook.

## What you can post

| | |
|---|---|
| Text | up to 2000 characters; `@everyone`, role and user mentions are shown as text and do not notify anyone |
| Images | up to 10 per post, 10 MB each |
| Delete | removes the message the webhook created |
| Not available | titles, video, threads, embeds, scheduling on Discord's side |

Each published post gets a link `https://discord.com/channels/<server>/<channel>/<message>`.

## When something fails

| You see | Meaning | Do |
|---|---|---|
| `SOCIAL_ACCOUNT_EXPIRED` | The webhook was deleted or its token changed (HTTP 401, or 404 with Discord code 10015) | Create a new webhook and reconnect |
| Post retried later | Discord rate limit (429) or a Discord outage (5xx); SocialOS waits as long as Discord asks | Nothing |
| Post in `needs_review` | The request timed out after it was sent. Discord cannot tell SocialOS whether the message arrived, and re-sending could post it twice | Look at the channel, then decide whether to retry |

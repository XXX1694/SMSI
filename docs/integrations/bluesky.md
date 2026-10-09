# Bluesky setup guide

For users of a SocialOS instance. Nothing to configure on the server: Bluesky needs no developer app.

## Connect

1. In Bluesky open **Settings > Privacy and security > App passwords > Add App Password**. Name it "SocialOS" and copy the
   password (`xxxx-xxxx-xxxx-xxxx`). Do not use your main password. Revoking the app password disconnects SocialOS at once.
2. In SocialOS open **Accounts > Connect > Bluesky**, enter your handle (for example `name.bsky.social`) and the app password.
   Leave "Server" empty unless you host your own PDS (then enter its https address, without a path).
3. SocialOS logs in once to check the password and stores only your DID and handle as metadata. The app password is encrypted
   at rest. Session tokens are kept in server memory only and are never saved or logged.

The API key that connects needs the critical scope `social:connect`.

## What publishing does

- Text up to **300 graphemes**. SocialOS checks 300 characters, which is slightly stricter for emoji sequences.
- Links and `#hashtags` become clickable. Mentions are not linked.
- Up to **4 images**, 2 MB each. Alt text is not supported yet.
- Delete removes the post. Replies, video and native scheduling are not supported.
- If a publish times out, SocialOS looks the post up by its fixed record key before retrying, so it is not posted twice.

## Limits and problems

- Bluesky allows 30 logins per 5 minutes and 300 per day per account [V]. SocialOS reuses sessions, so this only matters if you
  reconnect repeatedly. On a rate limit the post is retried after the time Bluesky gives.
- "Reconnect needed" means the app password was revoked or changed. Create a new one and reconnect.
- Bluesky discourages app passwords for new apps; OAuth connect is planned.

## Check it by hand (owner)

```bash
# Verify the credentials with the API (creates no post)
curl -s -X POST "$API/api/v1/social/accounts/token" -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"provider":"bluesky","fields":{"handle":"name.bsky.social","app_password":"xxxx-xxxx-xxxx-xxxx"}}'
```

Publishing to a real account is public: create the test post on a private or throwaway account first.

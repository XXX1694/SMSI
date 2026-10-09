-- +goose Up
-- One-time codes that prove a Steerpost user controls a Telegram channel/group:
-- the user posts the code in the chat, the bot sees it and links the chat to
-- the code's owner. Only the SHA-256 of the code is stored.
CREATE TABLE telegram_link_codes (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash         text NOT NULL UNIQUE,
  expires_at        timestamptz NOT NULL,
  used_at           timestamptz,
  chat_id           text NOT NULL DEFAULT '',
  social_account_id uuid REFERENCES social_accounts(id) ON DELETE SET NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
-- Listing / limiting a user's active codes, newest first.
CREATE INDEX telegram_link_codes_user_idx ON telegram_link_codes(user_id, created_at DESC);
-- Active (unredeemed) codes per user; expiry is checked against the clock at query time.
CREATE INDEX telegram_link_codes_user_active_idx ON telegram_link_codes(user_id, expires_at) WHERE used_at IS NULL;
-- Purging stale rows.
CREATE INDEX telegram_link_codes_expires_idx ON telegram_link_codes(expires_at);
CREATE TRIGGER telegram_link_codes_updated_at BEFORE UPDATE ON telegram_link_codes
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS telegram_link_codes;

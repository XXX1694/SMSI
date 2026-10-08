-- +goose Up
-- Schema for the "ready for users" work: email verification and password reset
-- (email_tokens, users.email_verified_at), terms acceptance, plans and quotas
-- (users.plan, posts.quota_counted_at), account deletion (users.status 'deleted',
-- account_deletions) and data export (data_exports). All of it lives in this one
-- migration so later changes need none.
ALTER TABLE users
  ADD COLUMN email_verified_at timestamptz,
  ADD COLUMN terms_accepted_at timestamptz,
  ADD COLUMN terms_version     text NOT NULL DEFAULT '',
  ADD COLUMN plan              text NOT NULL DEFAULT 'free',
  ADD COLUMN deleted_at        timestamptz;
-- 00001_init.sql names the inline CHECK users_status_check (verified with \d users).
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active','disabled','deleted'));
CREATE INDEX users_deleted_idx ON users(deleted_at) WHERE status = 'deleted';

-- One-time tokens mailed to a user. Only the SHA-256 of the token is stored.
CREATE TABLE email_tokens (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  purpose    text NOT NULL CHECK (purpose IN ('verify_email','reset_password')),
  token_hash text NOT NULL UNIQUE,
  email      citext NOT NULL,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_tokens_user_purpose_idx ON email_tokens(user_id, purpose, created_at DESC);
CREATE INDEX email_tokens_expires_idx ON email_tokens(expires_at);

ALTER TABLE posts ADD COLUMN quota_counted_at timestamptz;
CREATE INDEX posts_user_quota_idx ON posts(user_id, quota_counted_at) WHERE quota_counted_at IS NOT NULL;

CREATE TABLE data_exports (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','ready','failed','expired')),
  storage_key text NOT NULL DEFAULT '',
  size_bytes  bigint NOT NULL DEFAULT 0,
  error_code  text NOT NULL DEFAULT '',
  expires_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX data_exports_user_idx ON data_exports(user_id, created_at DESC);
CREATE UNIQUE INDEX data_exports_active_uniq ON data_exports(user_id) WHERE status IN ('pending','running');

-- Deletion record kept after the user row is gone: no FK and no PII on purpose.
CREATE TABLE account_deletions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL,
  requested_at timestamptz NOT NULL,
  purged_at    timestamptz,
  counts       jsonb NOT NULL DEFAULT '{}',
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER email_tokens_updated_at      BEFORE UPDATE ON email_tokens      FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER data_exports_updated_at      BEFORE UPDATE ON data_exports      FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER account_deletions_updated_at BEFORE UPDATE ON account_deletions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS account_deletions, data_exports, email_tokens;
DROP INDEX IF EXISTS posts_user_quota_idx;
ALTER TABLE posts DROP COLUMN IF EXISTS quota_counted_at;
-- A 'deleted' row would violate the old constraint; fold it into 'disabled' first.
UPDATE users SET status = 'disabled' WHERE status = 'deleted';
DROP INDEX IF EXISTS users_deleted_idx;
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('active','disabled'));
ALTER TABLE users
  DROP COLUMN email_verified_at, DROP COLUMN terms_accepted_at, DROP COLUMN terms_version,
  DROP COLUMN plan, DROP COLUMN deleted_at;

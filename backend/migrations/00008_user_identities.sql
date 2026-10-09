-- +goose Up
SET LOCAL lock_timeout = '5s';
-- Social sign-in (D-023): which external accounts (Google, GitHub) may sign in as which user, and the short-lived
-- state of a sign-in round trip. Provider access tokens are never stored: they are used once and dropped.

-- One row per (provider, subject). subject is the provider's stable id (Google "sub", GitHub numeric id), never an
-- email address or a login name, both of which can change hands.
CREATE TABLE user_identities (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider      text NOT NULL CHECK (provider IN ('google','github')),
  subject       text NOT NULL,
  email         citext NOT NULL DEFAULT '',
  email_verified boolean NOT NULL DEFAULT false,
  linked_at     timestamptz NOT NULL,
  last_login_at timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  -- Serves the login lookup, and makes one external account belong to exactly one user.
  UNIQUE (provider, subject),
  -- Serves "identities of this user" and allows one identity per provider and user.
  UNIQUE (user_id, provider)
);

-- A sign-in or link attempt: first the state (CSRF, nonce and PKCE verifier for the callback), then, for a new user,
-- the pending sign-up ticket that /signup/complete redeems. Only hashes of state, nonce and ticket are stored;
-- the PKCE verifier is stored encrypted.
CREATE TABLE auth_oauth_flows (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider          text NOT NULL CHECK (provider IN ('google','github')),
  intent            text NOT NULL CHECK (intent IN ('login','link')),
  link_user_id      uuid REFERENCES users(id) ON DELETE CASCADE,
  state_hash        text NOT NULL UNIQUE,
  nonce_hash        text NOT NULL,
  code_verifier_enc text NOT NULL,
  redirect_after    text NOT NULL DEFAULT '',
  expires_at        timestamptz NOT NULL,
  used_at           timestamptz,
  ticket_hash       text UNIQUE,
  ticket_expires_at timestamptz,
  pending           jsonb,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  CHECK ((intent = 'link') = (link_user_id IS NOT NULL))
);
-- Serves the purge of expired flows.
CREATE INDEX auth_oauth_flows_exp_idx ON auth_oauth_flows(expires_at);
-- Serves the foreign key: deleting a user looks up the flows that link to them.
CREATE INDEX auth_oauth_flows_link_user_idx ON auth_oauth_flows(link_user_id) WHERE link_user_id IS NOT NULL;

CREATE TRIGGER user_identities_updated_at BEFORE UPDATE ON user_identities
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER auth_oauth_flows_updated_at BEFORE UPDATE ON auth_oauth_flows
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A user who signed up through a provider has no password until they set one.
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
-- Users that an earlier Down marked with "!" (see below) have no password again.
UPDATE users SET password_hash = NULL WHERE password_hash = '!';

-- +goose Down
SET LOCAL lock_timeout = '5s';
DROP TABLE IF EXISTS auth_oauth_flows;
DROP TABLE IF EXISTS user_identities;
-- NOT NULL needs a value for the password-less users. "!" is not a valid argon2 encoding, so it can never verify:
-- those users sign in again only after a password reset by mail.
UPDATE users SET password_hash = '!' WHERE password_hash IS NULL;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;

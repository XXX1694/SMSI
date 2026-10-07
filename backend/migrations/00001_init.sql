-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email         citext NOT NULL UNIQUE,
  password_hash text NOT NULL,
  display_name  text NOT NULL DEFAULT '',
  status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  text NOT NULL UNIQUE,
  csrf_token  text NOT NULL,
  expires_at  timestamptz NOT NULL,
  user_agent  text NOT NULL DEFAULT '',
  ip          text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions(user_id);
CREATE INDEX sessions_expires_idx ON sessions(expires_at);

CREATE TABLE oauth_states (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider       text NOT NULL,
  state_hash     text NOT NULL UNIQUE,
  code_verifier  text NOT NULL,
  redirect_after text NOT NULL DEFAULT '',
  expires_at     timestamptz NOT NULL,
  used_at        timestamptz,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE social_accounts (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider            text NOT NULL,
  provider_account_id text NOT NULL,
  username            text NOT NULL DEFAULT '',
  display_name        text NOT NULL DEFAULT '',
  avatar_url          text NOT NULL DEFAULT '',
  scopes              text[] NOT NULL DEFAULT '{}',
  metadata            jsonb NOT NULL DEFAULT '{}',
  status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active','expired','revoked','error')),
  connected_at        timestamptz NOT NULL DEFAULT now(),
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, provider, provider_account_id)
);
CREATE INDEX social_accounts_user_status_idx ON social_accounts(user_id, status);

CREATE TABLE oauth_credentials (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  social_account_id  uuid NOT NULL UNIQUE REFERENCES social_accounts(id) ON DELETE CASCADE,
  access_token_enc   text NOT NULL DEFAULT '',
  refresh_token_enc  text NOT NULL DEFAULT '',
  expires_at         timestamptz,
  refresh_expires_at timestamptz,
  key_version        text NOT NULL DEFAULT 'v1',
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE posts (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title          text NOT NULL DEFAULT '',
  content        text NOT NULL DEFAULT '',
  status         text NOT NULL DEFAULT 'draft'
                 CHECK (status IN ('draft','scheduled','publishing','published','partially_published','failed','cancelled')),
  scheduled_at   timestamptz,
  published_at   timestamptz,
  created_by     text NOT NULL DEFAULT 'user' CHECK (created_by IN ('user','api_key')),
  created_by_ref text NOT NULL DEFAULT '',
  deleted_at     timestamptz,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX posts_user_status_idx ON posts(user_id, status) WHERE deleted_at IS NULL;
CREATE INDEX posts_user_scheduled_idx ON posts(user_id, scheduled_at) WHERE deleted_at IS NULL;
CREATE INDEX posts_user_created_idx ON posts(user_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;

CREATE TABLE post_targets (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  post_id           uuid NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  social_account_id uuid NOT NULL REFERENCES social_accounts(id),
  platform          text NOT NULL,
  content           text NOT NULL DEFAULT '',
  status            text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','publishing','published','failed','cancelled','needs_review')),
  external_post_id  text NOT NULL DEFAULT '',
  external_url      text NOT NULL DEFAULT '',
  published_at      timestamptz,
  error_code        text NOT NULL DEFAULT '',
  error_message     text NOT NULL DEFAULT '',
  idempotency_key   text NOT NULL UNIQUE,
  attempt_count     int  NOT NULL DEFAULT 0,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  UNIQUE (post_id, social_account_id)
);
CREATE INDEX post_targets_post_idx ON post_targets(post_id);
CREATE INDEX post_targets_user_status_idx ON post_targets(user_id, status);
CREATE INDEX post_targets_publishing_idx ON post_targets(updated_at) WHERE status = 'publishing';

CREATE TABLE media (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind          text NOT NULL CHECK (kind IN ('image','video')),
  mime_type     text NOT NULL,
  size_bytes    bigint NOT NULL,
  storage_key   text NOT NULL UNIQUE,
  original_name text NOT NULL DEFAULT '',
  width         int NOT NULL DEFAULT 0,
  height        int NOT NULL DEFAULT 0,
  status        text NOT NULL DEFAULT 'ready',
  sha256        text NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX media_user_created_idx ON media(user_id, created_at DESC, id DESC);

CREATE TABLE post_media (
  post_id  uuid NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  media_id uuid NOT NULL REFERENCES media(id) ON DELETE RESTRICT,
  position int NOT NULL DEFAULT 0,
  PRIMARY KEY (post_id, media_id)
);
CREATE INDEX post_media_media_idx ON post_media(media_id);

CREATE TABLE scheduled_jobs (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  post_target_id uuid NOT NULL REFERENCES post_targets(id) ON DELETE CASCADE,
  run_at         timestamptz NOT NULL,
  asynq_task_id  text NOT NULL DEFAULT '',
  status         text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','enqueued','done','cancelled')),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX scheduled_jobs_active_uniq ON scheduled_jobs(post_target_id) WHERE status IN ('pending','enqueued');
CREATE INDEX scheduled_jobs_run_at_idx ON scheduled_jobs(run_at) WHERE status = 'pending';
CREATE INDEX scheduled_jobs_active_run_idx ON scheduled_jobs(run_at) WHERE status IN ('pending','enqueued');

CREATE TABLE publication_attempts (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  post_target_id    uuid NOT NULL REFERENCES post_targets(id) ON DELETE CASCADE,
  attempt_no        int NOT NULL,
  started_at        timestamptz NOT NULL DEFAULT now(),
  finished_at       timestamptz,
  status            text NOT NULL CHECK (status IN ('started','succeeded','failed','unknown')),
  error_code        text NOT NULL DEFAULT '',
  error_message     text NOT NULL DEFAULT '',
  response_metadata jsonb NOT NULL DEFAULT '{}',
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  UNIQUE (post_target_id, attempt_no)
);

CREATE TABLE api_keys (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         text NOT NULL,
  prefix       text NOT NULL,
  key_hash     text NOT NULL UNIQUE,
  scopes       text[] NOT NULL DEFAULT '{}',
  expires_at   timestamptz,
  revoked_at   timestamptz,
  last_used_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX api_keys_user_idx ON api_keys(user_id, created_at DESC);

CREATE TABLE mcp_connections (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  api_key_id   uuid NOT NULL UNIQUE REFERENCES api_keys(id) ON DELETE CASCADE,
  name         text NOT NULL,
  client_name  text NOT NULL DEFAULT '',
  last_seen_at timestamptz,
  revoked_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mcp_connections_user_idx ON mcp_connections(user_id, created_at DESC);

CREATE TABLE audit_logs (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  actor_type    text NOT NULL CHECK (actor_type IN ('user','api_key','scheduler','system')),
  actor_id      text NOT NULL DEFAULT '',
  actor_label   text NOT NULL DEFAULT '',
  action        text NOT NULL,
  resource_type text NOT NULL DEFAULT '',
  resource_id   text NOT NULL DEFAULT '',
  metadata      jsonb NOT NULL DEFAULT '{}',
  request_id    text NOT NULL DEFAULT '',
  ip            text NOT NULL DEFAULT '',
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_user_created_idx ON audit_logs(user_id, created_at DESC, id DESC);

CREATE TABLE analytics (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  social_account_id uuid NOT NULL REFERENCES social_accounts(id) ON DELETE CASCADE,
  post_target_id    uuid REFERENCES post_targets(id) ON DELETE SET NULL,
  metric            text NOT NULL,
  value             bigint NOT NULL DEFAULT 0,
  captured_at       timestamptz NOT NULL DEFAULT now(),
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX analytics_user_captured_idx ON analytics(user_id, captured_at);

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['users','sessions','oauth_states','social_accounts','oauth_credentials','posts',
    'post_targets','media','scheduled_jobs','publication_attempts','api_keys','mcp_connections','audit_logs','analytics']
  LOOP
    EXECUTE format('CREATE TRIGGER %I_updated_at BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t, t);
  END LOOP;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS analytics, audit_logs, mcp_connections, api_keys, publication_attempts, scheduled_jobs,
  post_media, media, post_targets, posts, oauth_credentials, social_accounts, oauth_states, sessions, users CASCADE;
DROP FUNCTION IF EXISTS set_updated_at();

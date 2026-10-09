-- +goose Up
-- Owner approvals for dangerous actions attempted with an API key (D-013), and the per-key policy that
-- decides whether they are needed. Existing keys get 'approve' too: they are the ones an agent holds today.
ALTER TABLE api_keys ADD COLUMN dangerous_policy text NOT NULL DEFAULT 'approve'
  CHECK (dangerous_policy IN ('approve','trusted'));

CREATE TABLE action_approvals (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  actor_type    text NOT NULL CHECK (actor_type IN ('api_key')),
  actor_id      text NOT NULL,
  actor_label   text NOT NULL DEFAULT '',
  action        text NOT NULL CHECK (action IN ('post.publish','post.retry_now','post.delete',
                                                'social_account.disconnect','social_account.connect_token',
                                                'post.schedule_soon')),
  resource_type text NOT NULL,
  resource_id   text NOT NULL DEFAULT '',
  fingerprint   text NOT NULL,
  summary       jsonb NOT NULL DEFAULT '{}',
  status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','denied','consumed','expired')),
  expires_at    timestamptz NOT NULL,
  decided_at    timestamptz,
  consumed_at   timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
-- Serves the owner's list (user, newest first, status filter) and the per-user pending count.
CREATE INDEX action_approvals_user_idx ON action_approvals(user_id, status, created_at DESC);

CREATE TRIGGER action_approvals_updated_at BEFORE UPDATE ON action_approvals
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS action_approvals;
ALTER TABLE api_keys DROP COLUMN dangerous_policy;

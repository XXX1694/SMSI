-- +goose Up
-- Account deletion with a grace period (D-019). users.status 'deleted', users.deleted_at and account_deletions already
-- exist (00003); this adds the schedule and the indexes the batched purge needs.
ALTER TABLE users ADD COLUMN deletion_scheduled_at timestamptz;
-- Serves the worker sweep for accounts whose grace period is over.
CREATE INDEX users_deletion_due_idx ON users(deletion_scheduled_at) WHERE deletion_scheduled_at IS NOT NULL;

-- One deletion record per user: a new request replaces the record of a cancelled one.
CREATE UNIQUE INDEX account_deletions_user_uniq ON account_deletions(user_id);

-- Foreign keys the purge cascades through had no index on the referencing column: every deleted post_target would
-- scan scheduled_jobs and analytics once.
CREATE INDEX scheduled_jobs_target_idx ON scheduled_jobs(post_target_id);
CREATE INDEX analytics_target_idx ON analytics(post_target_id) WHERE post_target_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS analytics_target_idx;
DROP INDEX IF EXISTS scheduled_jobs_target_idx;
DROP INDEX IF EXISTS account_deletions_user_uniq;
DROP INDEX IF EXISTS users_deletion_due_idx;
ALTER TABLE users DROP COLUMN deletion_scheduled_at;

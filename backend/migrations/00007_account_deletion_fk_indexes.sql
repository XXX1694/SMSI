-- +goose NO TRANSACTION
-- +goose Up
-- Foreign keys the account purge (00006, D-019) cascades through had no index on the referencing column: every deleted
-- post_target would scan scheduled_jobs and analytics once. Built CONCURRENTLY (hence NO TRANSACTION) so the build does
-- not block writes to these live tables.
CREATE INDEX CONCURRENTLY IF NOT EXISTS scheduled_jobs_target_idx ON scheduled_jobs(post_target_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS analytics_target_idx ON analytics(post_target_id) WHERE post_target_id IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS analytics_target_idx;
DROP INDEX CONCURRENTLY IF EXISTS scheduled_jobs_target_idx;

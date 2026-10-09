-- +goose NO TRANSACTION
-- +goose Up
-- Keyset paging by id for the account data export (D-018): each batch reads "this user's rows after id X".
-- Without a (user_id, id) index every batch would sort all of the user's rows again. Built CONCURRENTLY (hence NO
-- TRANSACTION) so a large audit_logs or posts table is not locked for writes while the index is made.
CREATE INDEX CONCURRENTLY IF NOT EXISTS posts_user_id_idx ON posts(user_id, id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS audit_logs_user_id_idx ON audit_logs(user_id, id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS media_user_id_idx ON media(user_id, id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS action_approvals_user_id_idx ON action_approvals(user_id, id);
-- Serves the export sweep (due, expired and stuck rows) without scanning every export.
CREATE INDEX CONCURRENTLY IF NOT EXISTS data_exports_sweep_idx ON data_exports(status, updated_at);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS data_exports_sweep_idx;
DROP INDEX CONCURRENTLY IF EXISTS action_approvals_user_id_idx;
DROP INDEX CONCURRENTLY IF EXISTS media_user_id_idx;
DROP INDEX CONCURRENTLY IF EXISTS audit_logs_user_id_idx;
DROP INDEX CONCURRENTLY IF EXISTS posts_user_id_idx;

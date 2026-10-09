-- +goose Up
-- Keyset paging by id for the account data export (D-018): each batch reads "this user's rows after id X".
-- Without a (user_id, id) index every batch would sort all of the user's rows again.
CREATE INDEX posts_user_id_idx ON posts(user_id, id);
CREATE INDEX audit_logs_user_id_idx ON audit_logs(user_id, id);
CREATE INDEX media_user_id_idx ON media(user_id, id);
CREATE INDEX action_approvals_user_id_idx ON action_approvals(user_id, id);
-- Serves the export sweep (due, expired and stuck rows) without scanning every export.
CREATE INDEX data_exports_sweep_idx ON data_exports(status, updated_at);

-- +goose Down
DROP INDEX IF EXISTS data_exports_sweep_idx;
DROP INDEX IF EXISTS action_approvals_user_id_idx;
DROP INDEX IF EXISTS media_user_id_idx;
DROP INDEX IF EXISTS audit_logs_user_id_idx;
DROP INDEX IF EXISTS posts_user_id_idx;

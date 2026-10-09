package postgres

import (
	"context"
	"log/slog"

	"github.com/pressly/goose/v3"
)

// MigrateDownTo rolls the schema back until version is the newest applied one, so a migration test can name the
// version it examines instead of counting "down" steps that move whenever another migration lands.
func MigrateDownTo(ctx context.Context, url string, version int64, log *slog.Logger) error {
	db, err := openGoose(url, log)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return goose.DownToContext(ctx, db, ".", version)
}

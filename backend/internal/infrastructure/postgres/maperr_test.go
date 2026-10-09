package postgres

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/socialos/backend/internal/domain/errs"
)

func TestMapErrTurnsDeadlockAndSerializationIntoConflict(t *testing.T) {
	for _, code := range []string{"40P01", "40001"} {
		err := mapErr(fmt.Errorf("update: %w", &pgconn.PgError{Code: code}), "post")
		if !errs.Is(err, errs.Conflict) {
			t.Errorf("%s mapped to %v, want CONFLICT", code, err)
		}
	}
}

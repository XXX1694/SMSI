package postgres

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
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

// brokenRows ends iteration with a database error, as Postgres does when it aborts a statement mid-stream.
type brokenRows struct {
	pgx.Rows
	err error
}

func (brokenRows) Close()       {}
func (brokenRows) Next() bool   { return false }
func (b brokenRows) Err() error { return b.err }

func TestCollectJobsMapsDeadlockFromRows(t *testing.T) {
	_, err := collectJobs(brokenRows{err: &pgconn.PgError{Code: "40P01"}})
	if !errs.Is(err, errs.Conflict) {
		t.Fatalf("got %v, want CONFLICT", err)
	}
	if e, _ := errs.As(err); !e.Transient {
		t.Fatal("deadlock conflict must be marked transient")
	}
}

package postgres_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/socialos/backend/internal/infrastructure/postgres"
	"github.com/socialos/backend/internal/testutil"
)

// schemaSnapshot lists every column, constraint and index of the public schema in a stable order.
// goose's own version table is excluded: it legitimately differs between runs.
func schemaSnapshot(t *testing.T, url string) map[string][]string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	queries := map[string]string{
		"columns": `SELECT table_name||'.'||column_name||' '||data_type||' null='||is_nullable||' default='||COALESCE(column_default,'')
			FROM information_schema.columns WHERE table_schema='public' AND table_name<>'goose_db_version' ORDER BY 1`,
		"constraints": `SELECT c.conrelid::regclass::text||' '||c.conname||' '||pg_get_constraintdef(c.oid)
			FROM pg_constraint c JOIN pg_namespace n ON n.oid=c.connamespace
			WHERE n.nspname='public' AND c.conrelid::regclass::text<>'goose_db_version' ORDER BY 1`,
		"indexes": `SELECT tablename||' '||indexname||' '||indexdef FROM pg_indexes
			WHERE schemaname='public' AND tablename<>'goose_db_version' ORDER BY 1`,
	}
	out := map[string][]string{}
	for name, q := range queries {
		rows, err := conn.Query(ctx, q)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out[name] = append(out[name], s)
		}
		rows.Close()
		if rows.Err() != nil || len(out[name]) == 0 {
			t.Fatalf("%s: err=%v rows=%d", name, rows.Err(), len(out[name]))
		}
	}
	return out
}

// Up to latest, down to 0, up again: the schema must come back identical.
// FreshDatabase creates a dedicated throwaway database (dropped on cleanup), so other tests are unaffected.
func TestMigrationsRoundTrip(t *testing.T) {
	url := testutil.FreshDatabase(t) // already migrated up
	ctx := context.Background()
	first := schemaSnapshot(t, url)

	if err := postgres.Migrate(ctx, url, "reset", testutil.Logger()); err != nil {
		t.Fatalf("down to 0: %v", err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	var left int
	err = conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND table_name<>'goose_db_version'`).Scan(&left)
	_ = conn.Close(ctx)
	if err != nil || left != 0 {
		t.Fatalf("after down to 0: %d tables left (err=%v)", left, err)
	}

	if err := postgres.Migrate(ctx, url, "up", testutil.Logger()); err != nil {
		t.Fatalf("second up: %v", err)
	}
	second := schemaSnapshot(t, url)
	for _, kind := range []string{"columns", "constraints", "indexes"} {
		if !reflect.DeepEqual(first[kind], second[kind]) {
			t.Errorf("%s differ after up/down/up:\nfirst:  %q\nsecond: %q", kind, first[kind], second[kind])
		}
	}
}

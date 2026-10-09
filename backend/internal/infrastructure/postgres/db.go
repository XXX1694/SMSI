// Package postgres implements the application repositories with pgx and
// hand-written SQL. Every query on a user-owned table filters by user_id
// (tenant isolation); the few worker-internal lookups by primary key are
// marked "system" and are never reachable from the HTTP layer.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/socialos/backend/internal/domain/errs"
)

// DB wraps a pgx pool and carries transactions through context.
type DB struct {
	Pool *pgxpool.Pool
}

// Open connects and pings the database.
func Open(ctx context.Context, url string, maxConns int32) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse url: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Close releases the pool.
func (d *DB) Close() { d.Pool.Close() }

// Ping checks connectivity (readiness probe).
func (d *DB) Ping(ctx context.Context) error { return d.Pool.Ping(ctx) }

type txKey struct{}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// q returns the transaction in ctx, or the pool.
func (d *DB) q(ctx context.Context) querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return d.Pool
}

// WithoutTx returns ctx without its transaction: queries made with it use the pool and commit on their own. Used for
// records that must outlive the rollback of the request that produced them (a pending approval).
func (d *DB) WithoutTx(ctx context.Context) context.Context {
	return context.WithValue(ctx, txKey{}, nil)
}

// InTx runs fn inside a transaction. Nested calls join the outer transaction.
func (d *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	return nil
}

// mapErr converts driver errors into domain errors.
func mapErr(err error, resource string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.NotFoundf(resource)
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return errs.Wrap(errs.Conflict, resource+" already exists", err)
		case "23503":
			return errs.Wrap(errs.Validation, "referenced "+resource+" does not exist", err)
		case "22P02":
			return errs.Wrap(errs.Validation, "invalid identifier", err)
		}
	}
	return err
}

// mustAffect returns NOT_FOUND when an update touched no row (tenant mismatch).
func mustAffect(tag pgconn.CommandTag, err error, resource string) error {
	if err != nil {
		return mapErr(err, resource)
	}
	if tag.RowsAffected() == 0 {
		return errs.NotFoundf(resource)
	}
	return nil
}

package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaLockSQL takes a transaction-scoped advisory lock so concurrent
// EnsureSchema calls (several processes booting against a fresh database)
// run one at a time: a bare CREATE TABLE IF NOT EXISTS is not safe under
// that race and fails on pg_type_typname_nsp_index. Same key as the GORM
// HistoryStore, since both manage the same table.
const schemaLockSQL = `SELECT pg_advisory_xact_lock(hashtext('donkey.seeder_history'))`

const createTableSQL = `
CREATE TABLE IF NOT EXISTS public.seeder_history (
    seeder_id   VARCHAR(255) PRIMARY KEY,
    executed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

type HistoryStore struct {
	db Querier
}

func NewHistoryStore(db Querier) *HistoryStore {
	return &HistoryStore{db: db}
}

func (h *HistoryStore) EnsureSchema(ctx context.Context) error {
	db, ok := h.db.(interface {
		BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return createSchema(ctx, h.db)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ensure schema: begin: %w", err)
	}
	if err := createSchema(ctx, tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func createSchema(ctx context.Context, q Querier) error {
	if _, err := q.ExecContext(ctx, schemaLockSQL); err != nil {
		return fmt.Errorf("ensure schema: lock: %w", err)
	}
	_, err := q.ExecContext(ctx, createTableSQL)
	return err
}

func (h *HistoryStore) HasRun(ctx context.Context, id string) (bool, error) {
	var count int
	err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM public.seeder_history WHERE seeder_id = $1`, id,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (h *HistoryStore) MarkRun(ctx context.Context, id string) error {
	_, err := h.db.ExecContext(ctx,
		`INSERT INTO public.seeder_history (seeder_id) VALUES ($1)`, id,
	)
	return err
}

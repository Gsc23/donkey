package postgres

import "context"

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
	_, err := h.db.ExecContext(ctx, createTableSQL)
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

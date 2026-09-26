package gormadapter

import (
	"context"

	"gorm.io/gorm"
)

const createTableSQL = `
CREATE TABLE IF NOT EXISTS public.seeder_history (
    seeder_id   VARCHAR(255) PRIMARY KEY,
    executed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

type HistoryStore struct{ db *gorm.DB }

func NewHistoryStore(db *gorm.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

func (h *HistoryStore) EnsureSchema(ctx context.Context) error {
	return h.db.WithContext(ctx).Exec(createTableSQL).Error
}

func (h *HistoryStore) HasRun(ctx context.Context, id string) (bool, error) {
	var count int64
	err := h.db.WithContext(ctx).
		Table("public.seeder_history").
		Where("seeder_id = ?", id).
		Count(&count).Error
	return count > 0, err
}

func (h *HistoryStore) MarkRun(ctx context.Context, id string) error {
	return h.db.WithContext(ctx).
		Exec(`INSERT INTO public.seeder_history (seeder_id) VALUES (?)`, id).Error
}

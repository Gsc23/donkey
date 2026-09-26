package pgseeders

import (
	"context"

	"github.com/Gsc23/donkey/adapter/postgres"
)

type CreateAdmin struct{}

func (CreateAdmin) ID() string             { return "2026-09-26-create-admin-pg" }
func (CreateAdmin) Dependencies() []string { return nil }

func (CreateAdmin) Run(ctx context.Context, db postgres.Querier) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS pg_users (
			id   SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE
		)
	`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO pg_users (name) VALUES ('admin')
		ON CONFLICT (name) DO NOTHING
	`)
	return err
}

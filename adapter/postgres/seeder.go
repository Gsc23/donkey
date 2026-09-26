package postgres

import (
	"context"
	"database/sql"

	"github.com/Gsc23/donkey/core"
)

type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Seeder interface {
	core.Identifiable
	Run(ctx context.Context, db Querier) error
}

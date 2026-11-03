package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Gsc23/donkey/core"
)

type Adapter struct {
	db *sql.DB
}

func NewAdapter(db *sql.DB) *Adapter {
	return &Adapter{db: db}
}

func (a *Adapter) Execute(ctx context.Context, s core.Identifiable) error {
	ps, ok := s.(Seeder)
	if !ok {
		return fmt.Errorf("seeder %q does not implement postgres.Seeder", s.ID())
	}
	return ps.Run(ctx, a.db)
}

func (a *Adapter) Transaction(ctx context.Context, fn func(context.Context, core.Scope) error) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	scope := core.Scope{
		Adapter: &txAdapter{tx: tx},
		History: NewHistoryStore(tx),
	}
	if err := fn(ctx, scope); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

type txAdapter struct {
	tx *sql.Tx
}

func (a *txAdapter) Execute(ctx context.Context, s core.Identifiable) error {
	ps, ok := s.(Seeder)
	if !ok {
		return fmt.Errorf("seeder %q does not implement postgres.Seeder", s.ID())
	}
	return ps.Run(ctx, a.tx)
}

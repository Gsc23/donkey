package gormadapter

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/Gsc23/donkey/core"
)

type GORMAdapter struct {
	db *gorm.DB
}

func New(db *gorm.DB) *GORMAdapter {
	return &GORMAdapter{db: db}
}

func (a *GORMAdapter) Execute(ctx context.Context, s core.Identifiable) error {
	gs, ok := s.(Seeder)
	if !ok {
		return fmt.Errorf("seeder %q não implementa gormadapter.Seeder", s.ID())
	}
	return gs.Run(ctx, a.db.WithContext(ctx))
}

func (a *GORMAdapter) Transaction(ctx context.Context, fn func(context.Context, core.Scope) error) error {
	return a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope := core.Scope{
			Adapter: &GORMAdapter{db: tx},
			History: NewHistoryStore(tx),
		}
		return fn(ctx, scope)
	})
}

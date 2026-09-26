package gormadapter

import (
	"context"

	"gorm.io/gorm"

	"github.com/Gsc23/donkey/core"
)

type Seeder interface {
	core.Identifiable
	Run(ctx context.Context, db *gorm.DB) error
}

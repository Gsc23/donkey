package seeders

import (
	"context"

	"gorm.io/gorm"
)

type User struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

type CreateAdmin struct{}

func (CreateAdmin) ID() string             { return "2026-09-26-create-admin" }
func (CreateAdmin) Dependencies() []string { return nil }

func (CreateAdmin) Run(ctx context.Context, db *gorm.DB) error {
	if err := db.WithContext(ctx).AutoMigrate(&User{}); err != nil {
		return err
	}
	return db.WithContext(ctx).FirstOrCreate(&User{Name: "admin"}, User{Name: "admin"}).Error
}

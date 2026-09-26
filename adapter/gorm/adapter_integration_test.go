//go:build integration

package gormadapter_test

import (
	"context"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	gormadapter "github.com/Gsc23/donkey/adapter/gorm"
	pgstore "github.com/Gsc23/donkey/adapter/postgres"
	"github.com/Gsc23/donkey/core"
	"github.com/Gsc23/donkey/example/seeders"
)

func TestIntegration_RunAndSkipOnSecondRun(t *testing.T) {
	dsn := "host=localhost user=seeder password=seeder dbname=seeder_dev port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}

	history := pgstore.NewHistoryStore(db)
	ctx := context.Background()
	if err := history.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	t.Cleanup(func() {
		db.WithContext(ctx).Exec(`DELETE FROM public.seeder_history WHERE seeder_id = ?`, seeders.CreateAdmin{}.ID())
	})

	adapter := gormadapter.New(db)
	toSeed := []core.Identifiable{seeders.CreateAdmin{}}
	runner := core.New(adapter, history, core.NoTransaction)

	plan1, err := core.NewPlanner(history).Compile(ctx, toSeed)
	if err != nil {
		t.Fatal(err)
	}
	if plan1.Steps[0].Action != core.ActionRun {
		t.Fatal("esperava ActionRun na primeira vez")
	}
	if err := runner.Run(ctx, plan1); err != nil {
		t.Fatal(err)
	}

	plan2, err := core.NewPlanner(history).Compile(ctx, toSeed)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.Steps[0].Action != core.ActionSkip {
		t.Fatal("esperava ActionSkip na segunda vez")
	}
}

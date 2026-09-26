//go:build integration

package cli_test

import (
	"context"
	"io"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	gormadapter "github.com/Gsc23/donkey/adapter/gorm"
	pgstore "github.com/Gsc23/donkey/adapter/postgres"
	"github.com/Gsc23/donkey/core"
	"github.com/Gsc23/donkey/core/cli"
	"github.com/Gsc23/donkey/example/seeders"
)

func openTestDB(t *testing.T) (*gorm.DB, *pgstore.HistoryStore) {
	t.Helper()
	dsn := "host=localhost user=seeder password=seeder dbname=seeder_dev port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}

	history := pgstore.NewHistoryStore(db)
	if err := history.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return db, history
}

func cleanupHistory(t *testing.T, db *gorm.DB, id string) {
	t.Helper()
	t.Cleanup(func() {
		db.Exec(`DELETE FROM public.seeder_history WHERE seeder_id = ?`, id)
	})
}

// TestIntegration_RunDryRun_DoesNotWriteHistory exists specifically to
// guard the bug found while wiring `run --dry-run`: swapping only the
// Adapter for noop.Adapter but keeping the real HistoryStore around for
// execution meant runSequential still called HistoryStore.MarkRun for every
// step it "executed" (noop.Execute always succeeds), silently persisting a
// row for a seeder that never actually ran. This test fails if that
// regression is reintroduced.
func TestIntegration_RunDryRun_DoesNotWriteHistory(t *testing.T) {
	db, history := openTestDB(t)
	id := seeders.CreateAdmin{}.ID()
	cleanupHistory(t, db, id)

	ctx := context.Background()
	adapter := gormadapter.New(db)
	toSeed := []core.Identifiable{seeders.CreateAdmin{}}
	runner := core.New(adapter, history, core.NoTransaction)

	cmd := cli.New(runner, toSeed)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"run", "--dry-run"})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("dry-run não deveria falhar: %v", err)
	}

	ran, err := history.HasRun(ctx, id)
	if err != nil {
		t.Fatalf("HasRun: %v", err)
	}
	if ran {
		t.Fatal("dry-run não deveria ter marcado o seeder como executado")
	}
}

// TestIntegration_Run_WritesHistory is the mirror-image check: a real
// (non dry-run) run must still mark history, proving the fix above didn't
// accidentally disable persistence altogether.
func TestIntegration_Run_WritesHistory(t *testing.T) {
	db, history := openTestDB(t)
	id := seeders.CreateAdmin{}.ID()
	cleanupHistory(t, db, id)

	ctx := context.Background()
	adapter := gormadapter.New(db)
	toSeed := []core.Identifiable{seeders.CreateAdmin{}}
	runner := core.New(adapter, history, core.NoTransaction)

	cmd := cli.New(runner, toSeed)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"run"})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("run não deveria falhar: %v", err)
	}

	ran, err := history.HasRun(ctx, id)
	if err != nil {
		t.Fatalf("HasRun: %v", err)
	}
	if !ran {
		t.Fatal("run real deveria ter marcado o seeder como executado")
	}
}

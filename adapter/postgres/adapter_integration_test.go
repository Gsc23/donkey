//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	pgadapter "github.com/Gsc23/donkey/adapter/postgres"
	"github.com/Gsc23/donkey/core"
)

const testDSN = "host=localhost user=seeder password=seeder dbname=seeder_dev port=5432 sslmode=disable"

const notesTable = "adapter_postgres_test_notes"

type noteSeeder struct {
	id      string
	deps    []string
	failing bool
}

func (s noteSeeder) ID() string             { return s.id }
func (s noteSeeder) Dependencies() []string { return s.deps }

func (s noteSeeder) Run(ctx context.Context, db pgadapter.Querier) error {
	if s.failing {
		return errors.New("boom")
	}
	_, err := db.ExecContext(ctx, `INSERT INTO `+notesTable+` (note_id) VALUES ($1)`, s.id)
	return err
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("abrir conexão: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func setupNotesTable(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+notesTable+` (note_id VARCHAR(255) PRIMARY KEY)`); err != nil {
		t.Fatalf("criar tabela de teste: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DROP TABLE IF EXISTS ` + notesTable)
	})
}

func notesWritten(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `SELECT note_id FROM `+notesTable+` ORDER BY note_id`)
	if err != nil {
		t.Fatalf("consultar notas: %v", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("ler nota: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func cleanupHistory(t *testing.T, db *sql.DB, ids ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, id := range ids {
			db.Exec(`DELETE FROM public.seeder_history WHERE seeder_id = $1`, id)
		}
	})
}

func TestIntegration_RunAndSkipOnSecondRun(t *testing.T) {
	db := openTestDB(t)
	setupNotesTable(t, db)

	history := pgadapter.NewHistoryStore(db)
	ctx := context.Background()
	if err := history.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	id := "2026-09-26-it-note-single"
	cleanupHistory(t, db, id)

	adapter := pgadapter.NewAdapter(db)
	toSeed := []core.Identifiable{noteSeeder{id: id}}
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
	if got := notesWritten(t, db); len(got) != 1 || got[0] != id {
		t.Fatalf("esperava nota %q gravada, got %v", id, got)
	}

	plan2, err := core.NewPlanner(history).Compile(ctx, toSeed)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.Steps[0].Action != core.ActionSkip {
		t.Fatal("esperava ActionSkip na segunda vez")
	}
}

func TestIntegration_PerSeeder_FailureIsIsolated(t *testing.T) {
	db := openTestDB(t)
	setupNotesTable(t, db)

	history := pgadapter.NewHistoryStore(db)
	ctx := context.Background()
	if err := history.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	idA := "2026-09-26-it-note-a"
	idB := "2026-09-26-it-note-b"
	idC := "2026-09-26-it-note-c"
	cleanupHistory(t, db, idA, idB, idC)

	adapter := pgadapter.NewAdapter(db)
	toSeed := []core.Identifiable{
		noteSeeder{id: idA},
		noteSeeder{id: idB, deps: []string{idA}, failing: true},
		noteSeeder{id: idC, deps: []string{idB}},
	}
	runner := core.New(adapter, history, core.PerSeeder)

	plan, err := core.NewPlanner(history).Compile(ctx, toSeed)
	if err != nil {
		t.Fatal(err)
	}
	err = runner.Run(ctx, plan)

	var partial *core.PartialRunError
	if !errors.As(err, &partial) {
		t.Fatalf("esperava PartialRunError, got %v", err)
	}
	if partial.FailedID != idB || partial.Completed != 1 {
		t.Fatalf("PartialRunError inesperado: %+v", partial)
	}

	if got := notesWritten(t, db); len(got) != 1 || got[0] != idA {
		t.Fatalf("esperava só a nota %q persistida, got %v", idA, got)
	}
	ranA, _ := history.HasRun(ctx, idA)
	ranB, _ := history.HasRun(ctx, idB)
	if !ranA {
		t.Fatal("esperava idA marcado no histórico (sua própria transação comitou)")
	}
	if ranB {
		t.Fatal("não esperava idB marcado no histórico (sua transação deveria ter sido desfeita)")
	}
}

func TestIntegration_All_FailureRollsBackEverything(t *testing.T) {
	db := openTestDB(t)
	setupNotesTable(t, db)

	history := pgadapter.NewHistoryStore(db)
	ctx := context.Background()
	if err := history.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	idA := "2026-09-26-it-all-note-a"
	idB := "2026-09-26-it-all-note-b"
	cleanupHistory(t, db, idA, idB)

	adapter := pgadapter.NewAdapter(db)
	toSeed := []core.Identifiable{
		noteSeeder{id: idA},
		noteSeeder{id: idB, deps: []string{idA}, failing: true},
	}
	runner := core.New(adapter, history, core.All)

	plan, err := core.NewPlanner(history).Compile(ctx, toSeed)
	if err != nil {
		t.Fatal(err)
	}
	err = runner.Run(ctx, plan)

	var partial *core.PartialRunError
	if errors.As(err, &partial) {
		t.Fatalf("não esperava PartialRunError no modo All, got %+v", partial)
	}
	if err == nil {
		t.Fatal("esperava erro no modo All")
	}

	if got := notesWritten(t, db); len(got) != 0 {
		t.Fatalf("esperava rollback total (nenhuma nota), got %v", got)
	}
	ranA, _ := history.HasRun(ctx, idA)
	if ranA {
		t.Fatal("esperava idA desfeito junto com o resto da transação")
	}
}

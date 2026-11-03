//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"

	pgadapter "github.com/Gsc23/donkey/adapter/postgres"
)

// Several processes booting against a fresh database all call EnsureSchema
// at once. A bare CREATE TABLE IF NOT EXISTS is not safe under that race in
// Postgres (it fails on pg_type_typname_nsp_index), so EnsureSchema must
// serialize itself.
func TestIntegration_EnsureSchema_ConcurrentOnFreshDatabase(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for round := 0; round < 5; round++ {
		if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS public.seeder_history`); err != nil {
			t.Fatalf("drop: %v", err)
		}

		const workers = 16
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		start := make(chan struct{})
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs <- pgadapter.NewHistoryStore(db).EnsureSchema(ctx)
			}()
		}
		close(start)
		wg.Wait()
		close(errs)

		for err := range errs {
			if err != nil {
				t.Fatalf("rodada %d: EnsureSchema concorrente falhou: %v", round, err)
			}
		}
	}
}

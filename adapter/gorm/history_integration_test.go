//go:build integration

package gormadapter_test

import (
	"context"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	gormadapter "github.com/Gsc23/donkey/adapter/gorm"
	"github.com/Gsc23/donkey/internal/testdb"
)

// See adapter/postgres's twin test: concurrent EnsureSchema on a fresh
// database must not fail.
func TestIntegration_EnsureSchema_ConcurrentOnFreshDatabase(t *testing.T) {
	db, err := gorm.Open(postgres.Open(testdb.DSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("conectar: %v", err)
	}
	ctx := context.Background()

	for round := 0; round < 5; round++ {
		if err := db.Exec(`DROP TABLE IF EXISTS public.seeder_history`).Error; err != nil {
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
				errs <- gormadapter.NewHistoryStore(db).EnsureSchema(ctx)
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

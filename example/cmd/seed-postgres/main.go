// Command seed-postgres is a minimal example of how a consumer application
// embeds the framework using the native (database/sql, no ORM) Postgres
// adapter instead of GORM — see example/cmd/seed for the GORM equivalent.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Gsc23/donkey/adapter/postgres"
	"github.com/Gsc23/donkey/core"
	"github.com/Gsc23/donkey/core/cli"
	"github.com/Gsc23/donkey/example/pgseeders"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=seeder password=seeder dbname=seeder_dev port=5432 sslmode=disable"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("abrir conexão: %v", err)
	}

	history := postgres.NewHistoryStore(db)
	if err := history.EnsureSchema(context.Background()); err != nil {
		log.Fatalf("preparar seeder_history: %v", err)
	}

	runner := core.New(postgres.NewAdapter(db), history, core.PerSeeder)

	toSeed := []core.Identifiable{
		pgseeders.CreateAdmin{},
	}

	cmd := cli.New(runner, toSeed, postgres.NewCmd())
	if err := cmd.Execute(); err != nil {
		var ec core.ExitCoder
		if errors.As(err, &ec) {
			os.Exit(ec.ExitCode())
		}
		os.Exit(1)
	}
}

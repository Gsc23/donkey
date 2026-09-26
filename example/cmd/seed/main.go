// Command seed is a minimal example of how a consumer application embeds
// the framework: it owns the database connection, wires the GORM adapter
// and the Postgres HistoryStore, and exposes core/cli as its own "seed"
// subcommand.
package main

import (
	"context"
	"errors"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	gormadapter "github.com/Gsc23/donkey/adapter/gorm"
	"github.com/Gsc23/donkey/core"
	"github.com/Gsc23/donkey/core/cli"
	"github.com/Gsc23/donkey/example/seeders"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=seeder password=seeder dbname=seeder_dev port=5432 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("conectar ao banco: %v", err)
	}

	history := gormadapter.NewHistoryStore(db)
	if err := history.EnsureSchema(context.Background()); err != nil {
		log.Fatalf("preparar seeder_history: %v", err)
	}

	runner := core.New(gormadapter.New(db), history, core.PerSeeder)

	toSeed := []core.Identifiable{
		seeders.CreateAdmin{},
	}

	cmd := cli.New(runner, toSeed, gormadapter.NewCmd())
	if err := cmd.Execute(); err != nil {
		var ec core.ExitCoder
		if errors.As(err, &ec) {
			os.Exit(ec.ExitCode())
		}
		os.Exit(1)
	}
}

//go:build integration

package testdb

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const image = "postgres:16"

var dsn string

func Run(m *testing.M) int {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		dsn = v
		return m.Run()
	}

	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, image,
		tcpostgres.WithDatabase("seeder_dev"),
		tcpostgres.WithUsername("seeder"),
		tcpostgres.WithPassword("seeder"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testdb: subir container %s: %v (Docker está rodando? ou defina DATABASE_URL)\n", image, err)
		return 1
	}
	defer func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			fmt.Fprintf(os.Stderr, "testdb: derrubar container: %v\n", err)
		}
	}()

	dsn, err = c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testdb: connection string: %v\n", err)
		return 1
	}
	return m.Run()
}

func DSN() string {
	if dsn == "" {
		panic("testdb.DSN chamado sem testdb.Run — adicione `func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }` ao pacote")
	}
	return dsn
}

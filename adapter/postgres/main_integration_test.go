//go:build integration

package postgres_test

import (
	"os"
	"testing"

	"github.com/Gsc23/donkey/internal/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

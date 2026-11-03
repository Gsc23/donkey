package gormadapter_test

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gormadapter "github.com/Gsc23/donkey/adapter/gorm"
	"github.com/Gsc23/donkey/core"
)

func TestScaffolder_GeneratesValidSeederFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "seeders")
	path, err := gormadapter.NewScaffolder().Scaffold(context.Background(), core.SeederSpec{
		ID:        "2026-11-02-create-orders",
		Slug:      "create-orders",
		Deps:      []string{"2026-01-01-create-admin"},
		CreatedAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		Dir:       dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler arquivo gerado: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), path, src, parser.AllErrors); err != nil {
		t.Fatalf("arquivo gerado não é Go válido: %v\n---\n%s", err, src)
	}

	content := string(src)
	for _, want := range []string{
		"package seeders",
		"type CreateOrders struct{}",
		`func (CreateOrders) ID() string             { return "2026-11-02-create-orders" }`,
		`func (CreateOrders) Dependencies() []string { return []string{"2026-01-01-create-admin"} }`,
		"func (CreateOrders) Run(ctx context.Context, db *gorm.DB) error {",
		`"gorm.io/gorm"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("esperava conter %q, got:\n%s", want, content)
		}
	}
}

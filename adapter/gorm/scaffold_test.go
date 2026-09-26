package gormadapter_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gormadapter "github.com/Gsc23/donkey/adapter/gorm"
)

func TestNewCmd_GeneratesValidGORMSeederFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "seeders")

	cmd := gormadapter.NewCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"create-orders", "--dir", dir, "--deps", "2026-01-01-create-admin"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := strings.TrimSpace(out.String())
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
		`Dependencies() []string { return []string{"2026-01-01-create-admin"} }`,
		"func (CreateOrders) Run(ctx context.Context, db *gorm.DB) error {",
		`"gorm.io/gorm"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("esperava conter %q, got:\n%s", want, content)
		}
	}
}

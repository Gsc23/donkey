package scaffold_test

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gsc23/donkey/internal/scaffold"
)

const fakeTemplate = `package {{.Package}}

type {{.TypeName}} struct{}

func ({{.TypeName}}) ID() string             { return "{{.ID}}" }
func ({{.TypeName}}) Dependencies() []string { return {{.Deps}} }
`

func runNewCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := scaffold.NewCmd("test", fakeTemplate)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func mustParseGo(t *testing.T, path string) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler arquivo gerado: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), path, src, parser.AllErrors); err != nil {
		t.Fatalf("arquivo gerado não é Go válido: %v\n---\n%s", err, src)
	}
}

func TestNewCmd_GeneratesValidFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "seeders")

	out, err := runNewCmd(t, "create-orders", "--dir", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := strings.TrimSpace(out)
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("esperava arquivo em %q: %v", path, statErr)
	}
	mustParseGo(t, path)

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "package seeders") {
		t.Fatalf("esperava package derivado do diretório 'seeders', got:\n%s", content)
	}
	if !strings.Contains(string(content), "type CreateOrders struct{}") {
		t.Fatalf("esperava type CreateOrders, got:\n%s", content)
	}
	wantID := time.Now().Format("2006-01-02") + "-create-orders"
	if !strings.Contains(string(content), `"`+wantID+`"`) {
		t.Fatalf("esperava ID %q, got:\n%s", wantID, content)
	}
	if !strings.Contains(string(content), "Dependencies() []string { return nil }") {
		t.Fatalf("esperava Dependencies() nil sem --deps, got:\n%s", content)
	}
}

func TestNewCmd_WithDeps(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "seeders")

	out, err := runNewCmd(t, "create-orders", "--dir", dir, "--deps", "2026-01-01-create-admin,2026-01-02-create-products")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	path := strings.TrimSpace(out)
	mustParseGo(t, path)

	content, _ := os.ReadFile(path)
	want := `Dependencies() []string { return []string{"2026-01-01-create-admin", "2026-01-02-create-products"} }`
	if !strings.Contains(string(content), want) {
		t.Fatalf("esperava %q, got:\n%s", want, content)
	}
}

func TestNewCmd_ExplicitPackageOverridesDirName(t *testing.T) {
	dir := t.TempDir() // basename is a random temp name, not a valid package hint

	out, err := runNewCmd(t, "create-orders", "--dir", dir, "--package", "myseeders")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	path := strings.TrimSpace(out)
	mustParseGo(t, path)

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "package myseeders") {
		t.Fatalf("esperava package myseeders, got:\n%s", content)
	}
}

func TestNewCmd_RejectsInvalidSlug(t *testing.T) {
	dir := t.TempDir()
	for _, slug := range []string{"CreateOrders", "create orders", "create_orders"} {
		if _, err := runNewCmd(t, slug, "--dir", dir); err == nil {
			t.Fatalf("esperava erro para slug inválido %q", slug)
		}
	}
}

func TestNewCmd_RejectsUnusablePackageName(t *testing.T) {
	dir := t.TempDir() // basename is not guaranteed to be a valid Go identifier

	if _, err := runNewCmd(t, "create-orders", "--dir", dir); err == nil {
		t.Fatal("esperava erro quando o nome do diretório não é um identificador Go válido")
	}
}

func TestNewCmd_DoesNotOverwriteExistingFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "seeders")

	if _, err := runNewCmd(t, "create-orders", "--dir", dir); err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}
	if _, err := runNewCmd(t, "create-orders", "--dir", dir); err == nil {
		t.Fatal("esperava erro ao gerar o mesmo seeder duas vezes no mesmo dia")
	}
}

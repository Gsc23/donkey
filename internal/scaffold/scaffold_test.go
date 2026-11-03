package scaffold_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/Gsc23/donkey/core"
	"github.com/Gsc23/donkey/internal/scaffold"
)

// Deliberately sloppy spacing: the Writer must gofmt whatever it renders.
var fakeTemplate = template.Must(template.New("fake").Parse(`package {{.Package}}
type {{.TypeName}} struct{}
func ({{.TypeName}}) ID() string {   return {{printf "%q" .ID}} }
func ({{.TypeName}}) Dependencies() []string { return {{if .Deps}}[]string{ {{- range $i, $d := .Deps}}{{if $i}}, {{end}}{{printf "%q" $d}}{{end -}} }{{else}}nil{{end}} }
`))

func spec(dir string) core.SeederSpec {
	return core.SeederSpec{
		ID:        "2026-11-02-create-orders",
		Slug:      "create-orders",
		CreatedAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		Dir:       dir,
	}
}

func scaffoldOK(t *testing.T, w *scaffold.Writer, s core.SeederSpec) (string, string) {
	t.Helper()
	path, err := w.Scaffold(context.Background(), s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler arquivo gerado: %v", err)
	}
	return path, string(src)
}

func TestScaffold_WritesGofmtedFileNamedByDateAndSlug(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "seeders")
	path, src := scaffoldOK(t, scaffold.New(fakeTemplate), spec(dir))

	if want := filepath.Join(dir, "20261102_create-orders.go"); path != want {
		t.Fatalf("esperava %q, got %q", want, path)
	}
	want := `package seeders

type CreateOrders struct{}

func (CreateOrders) ID() string             { return "2026-11-02-create-orders" }
func (CreateOrders) Dependencies() []string { return nil }
`
	if src != want {
		t.Fatalf("conteúdo inesperado:\n--- want\n%s--- got\n%s", want, src)
	}
}

func TestScaffold_RendersDeps(t *testing.T) {
	s := spec(filepath.Join(t.TempDir(), "seeders"))
	s.Deps = []string{"2026-01-01-create-admin", "2026-01-02-create-products"}
	_, src := scaffoldOK(t, scaffold.New(fakeTemplate), s)

	// Long enough that gofmt moves the body off the signature line.
	want := `return []string{"2026-01-01-create-admin", "2026-01-02-create-products"}`
	if !strings.Contains(src, want) {
		t.Fatalf("esperava %q, got:\n%s", want, src)
	}
}

func TestScaffold_ExplicitPackageOverridesDirName(t *testing.T) {
	s := spec(t.TempDir()) // basename is a random temp name, not a valid package hint
	s.Package = "myseeders"
	_, src := scaffoldOK(t, scaffold.New(fakeTemplate), s)

	if !strings.HasPrefix(src, "package myseeders\n") {
		t.Fatalf("esperava package myseeders, got:\n%s", src)
	}
}

func TestScaffold_RejectsUnusablePackageName(t *testing.T) {
	s := spec(filepath.Join(t.TempDir(), "my-seeders"))
	if _, err := scaffold.New(fakeTemplate).Scaffold(context.Background(), s); err == nil {
		t.Fatal("esperava erro quando o nome do diretório não é um identificador Go válido")
	}
}

func TestScaffold_DoesNotOverwriteExistingFile(t *testing.T) {
	s := spec(filepath.Join(t.TempDir(), "seeders"))
	w := scaffold.New(fakeTemplate)
	path, _ := scaffoldOK(t, w, s)
	if err := os.WriteFile(path, []byte("// editado pelo usuário\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Scaffold(context.Background(), s); err == nil {
		t.Fatal("esperava erro ao gerar o mesmo seeder duas vezes no mesmo dia")
	}
	if got, _ := os.ReadFile(path); string(got) != "// editado pelo usuário\n" {
		t.Fatalf("arquivo existente foi alterado: %q", got)
	}
}

func TestScaffold_InvalidRenderedGoLeavesNoFile(t *testing.T) {
	broken := template.Must(template.New("broken").Parse("package {{.Package}}\nfunc (\n"))
	dir := filepath.Join(t.TempDir(), "seeders")

	if _, err := scaffold.New(broken).Scaffold(context.Background(), spec(dir)); err == nil {
		t.Fatal("esperava erro para template que gera Go inválido")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("não deveria ter deixado arquivo para trás, got %v", entries)
	}
}

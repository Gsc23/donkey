// Package scaffold is the shared mechanics behind each adapter's
// core.Scaffolder: derive the package and type names, render the adapter's
// template, gofmt it and write it without overwriting anything. Adapters
// only supply the template.
package scaffold

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/Gsc23/donkey/core"
)

var identPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Data is what a seeder template receives.
type Data struct {
	Package  string
	TypeName string
	ID       string
	Deps     []string
}

type Writer struct {
	tpl *template.Template
}

var _ core.Scaffolder = (*Writer)(nil)

func New(tpl *template.Template) *Writer {
	return &Writer{tpl: tpl}
}

func (w *Writer) Scaffold(_ context.Context, spec core.SeederSpec) (string, error) {
	pkg := spec.Package
	if pkg == "" {
		pkg = filepath.Base(filepath.Clean(spec.Dir))
	}
	if !identPattern.MatchString(pkg) {
		return "", fmt.Errorf("invalid package name %q (derived from --dir=%q) — pass --package explicitly", pkg, spec.Dir)
	}

	// Render and gofmt fully in memory first: a template bug must never
	// leave a half-written file behind.
	var buf bytes.Buffer
	if err := w.tpl.Execute(&buf, Data{
		Package:  pkg,
		TypeName: toPascalCase(spec.Slug),
		ID:       spec.ID,
		Deps:     spec.Deps,
	}); err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("template produced invalid Go: %w\n---\n%s", err, buf.Bytes())
	}

	if err := os.MkdirAll(spec.Dir, 0o755); err != nil {
		return "", fmt.Errorf("create directory %q: %w", spec.Dir, err)
	}
	path := filepath.Join(spec.Dir, fmt.Sprintf("%s_%s.go", spec.CreatedAt.Format("20060102"), spec.Slug))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("%q already exists — delete it or pick another name", path)
		}
		return "", fmt.Errorf("create %q: %w", path, err)
	}
	if _, err := f.Write(src); err != nil {
		f.Close()
		os.Remove(path)
		return "", fmt.Errorf("write %q: %w", path, err)
	}
	return path, f.Close()
}

func toPascalCase(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

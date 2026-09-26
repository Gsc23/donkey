package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

var (
	slugPattern  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	identPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

func NewCmd(short, tplSrc string) *cobra.Command {
	var dir, pkg string
	var deps []string

	cmd := &cobra.Command{
		Use:   "new <nome>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := Generate(tplSrc, args[0], dir, pkg, deps)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "./seeders", "diretório onde o arquivo do seeder será criado")
	cmd.Flags().StringVar(&pkg, "package", "", "nome do pacote Go do arquivo gerado (default: nome do diretório de --dir)")
	cmd.Flags().StringSliceVar(&deps, "deps", nil, "IDs dos seeders dos quais este depende")
	return cmd
}

func Generate(tplSrc, slug, dir, pkg string, deps []string) (string, error) {
	if !slugPattern.MatchString(slug) {
		return "", fmt.Errorf("nome inválido %q: use apenas letras minúsculas, números e hífen (ex.: create-orders)", slug)
	}

	now := time.Now()
	id := fmt.Sprintf("%s-%s", now.Format("2006-01-02"), slug)
	typeName := toPascalCase(slug)

	packageName := pkg
	if packageName == "" {
		packageName = filepath.Base(filepath.Clean(dir))
	}
	if !identPattern.MatchString(packageName) {
		return "", fmt.Errorf("nome de pacote inválido %q (derivado de --dir=%q) — passe --package explicitamente", packageName, dir)
	}

	depsLiteral := "nil"
	if len(deps) > 0 {
		quoted := make([]string, len(deps))
		for i, d := range deps {
			quoted[i] = fmt.Sprintf("%q", d)
		}
		depsLiteral = fmt.Sprintf("[]string{%s}", strings.Join(quoted, ", "))
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("criar diretório %q: %w", dir, err)
	}

	path := filepath.Join(dir, fmt.Sprintf("%s_%s.go", now.Format("20060102"), slug))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("%q já existe — apague-o ou escolha outro nome", path)
		}
		return "", fmt.Errorf("criar %q: %w", path, err)
	}
	defer f.Close()

	tpl := template.Must(template.New("seeder").Parse(tplSrc))
	if err := tpl.Execute(f, map[string]string{
		"Package":  packageName,
		"TypeName": typeName,
		"ID":       id,
		"Deps":     depsLiteral,
	}); err != nil {
		return "", err
	}
	return path, nil
}

func toPascalCase(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

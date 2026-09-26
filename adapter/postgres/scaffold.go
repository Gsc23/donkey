package postgres

import (
	"github.com/spf13/cobra"

	"github.com/Gsc23/donkey/internal/scaffold"
)

const seederTemplate = `package {{.Package}}

import (
	"context"

	"github.com/Gsc23/donkey/adapter/postgres"
)

type {{.TypeName}} struct{}

func ({{.TypeName}}) ID() string             { return "{{.ID}}" }
func ({{.TypeName}}) Dependencies() []string { return {{.Deps}} }

func ({{.TypeName}}) Run(ctx context.Context, db postgres.Querier) error {
	// TODO: implementar
	return nil
}
`

func NewCmd() *cobra.Command {
	return scaffold.NewCmd("gera o boilerplate de um novo seeder Postgres (database/sql)", seederTemplate)
}

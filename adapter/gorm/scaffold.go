package gormadapter

import (
	"embed"
	"text/template"

	"github.com/Gsc23/donkey/core"
	"github.com/Gsc23/donkey/internal/scaffold"
)

//go:embed seeder.go.tmpl
var templates embed.FS

var seederTemplate = template.Must(template.ParseFS(templates, "seeder.go.tmpl"))

// NewScaffolder returns the core.Scaffolder that generates GORM seeders.
// Pass it to core.NewService via core.WithScaffolder to enable `seed new`.
func NewScaffolder() core.Scaffolder {
	return scaffold.New(seederTemplate)
}

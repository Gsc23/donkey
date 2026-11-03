package core

import (
	"context"
	"errors"
	"time"
)

var ErrScaffolderNotConfigured = errors.New("nenhum Scaffolder configurado — passe core.WithScaffolder(...) para core.NewService")

// NewSeederRequest is what a frontend asks for when creating a seeder.
type NewSeederRequest struct {
	Slug    string   // e.g. "create-orders"
	Deps    []string // IDs of already-registered seeders
	Dir     string   // where the file goes
	Package string   // Go package; empty lets the Scaffolder derive it from Dir
}

// SeederSpec is a validated seeder description, ready to be materialized.
// Every rule core owns (ID format, known dependencies, no ID collision) has
// already been checked when a Scaffolder receives it.
type SeederSpec struct {
	ID        string
	Slug      string
	Deps      []string
	CreatedAt time.Time
	Dir       string
	Package   string
}

// Scaffolder is the driven port that turns a SeederSpec into source code.
// Each adapter supplies one, since only it knows its Run signature.
type Scaffolder interface {
	Scaffold(ctx context.Context, spec SeederSpec) (path string, err error)
}

package noop

import (
	"context"
	"fmt"

	"github.com/Gsc23/donkey/core"
)

type Adapter struct {
	Log func(id string)
}

func New() *Adapter {
	return &Adapter{
		Log: func(id string) { fmt.Printf("[dry-run] executaria: %s\n", id) },
	}
}

func (a *Adapter) Execute(ctx context.Context, s core.Identifiable) error {
	a.Log(s.ID())
	return nil
}

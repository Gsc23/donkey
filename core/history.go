package core

import "context"

type HistoryStore interface {
	EnsureSchema(ctx context.Context) error
	HasRun(ctx context.Context, id string) (bool, error)
	MarkRun(ctx context.Context, id string) error
}

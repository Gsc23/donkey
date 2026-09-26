package core

import "context"

type Adapter interface {
	Execute(ctx context.Context, s Identifiable) error
}

type Scope struct {
	Adapter Adapter
	History HistoryStore
}

type TransactionalEngine interface {
	Adapter
	Transaction(ctx context.Context, fn func(ctx context.Context, scope Scope) error) error
}

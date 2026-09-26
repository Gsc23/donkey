package core

import (
	"context"
	"fmt"
)

type TransactionMode int

const (
	NoTransaction TransactionMode = iota
	PerSeeder
	All
)

type Runner struct {
	adapter Adapter
	history HistoryStore
	txMode  TransactionMode
}

func New(adapter Adapter, history HistoryStore, txMode TransactionMode) *Runner {
	return &Runner{adapter: adapter, history: history, txMode: txMode}
}

func (r *Runner) History() HistoryStore { return r.history }

func (r *Runner) Run(ctx context.Context, plan *ExecutionPlan) error {
	switch r.txMode {
	case All:
		return r.runAll(ctx, plan)
	case PerSeeder:
		return r.runPerSeeder(ctx, plan)
	default:
		return r.runSequential(ctx, plan)
	}
}

func (r *Runner) runSequential(ctx context.Context, plan *ExecutionPlan) error {
	for i, step := range plan.Steps {
		if step.Action == ActionSkip {
			continue
		}
		if err := r.adapter.Execute(ctx, step.Seeder); err != nil {
			return &PartialRunError{FailedID: step.Seeder.ID(), Completed: i, Err: err}
		}
		if r.history != nil {
			if err := r.history.MarkRun(ctx, step.Seeder.ID()); err != nil {
				return &PartialRunError{FailedID: step.Seeder.ID(), Completed: i, Err: err}
			}
		}
	}
	return nil
}

func (r *Runner) runPerSeeder(ctx context.Context, plan *ExecutionPlan) error {
	txEngine, ok := r.adapter.(TransactionalEngine)
	if !ok {
		return ErrTransactionNotSupported
	}

	for i, step := range plan.Steps {
		if step.Action == ActionSkip {
			continue
		}
		err := txEngine.Transaction(ctx, func(txCtx context.Context, scope Scope) error {
			if err := scope.Adapter.Execute(txCtx, step.Seeder); err != nil {
				return err
			}
			return scope.History.MarkRun(txCtx, step.Seeder.ID())
		})
		if err != nil {
			return &PartialRunError{FailedID: step.Seeder.ID(), Completed: i, Err: err}
		}
	}
	return nil
}

func (r *Runner) runAll(ctx context.Context, plan *ExecutionPlan) error {
	txEngine, ok := r.adapter.(TransactionalEngine)
	if !ok {
		return ErrTransactionNotSupported
	}
	return txEngine.Transaction(ctx, func(txCtx context.Context, scope Scope) error {
		for _, step := range plan.Steps {
			if step.Action == ActionSkip {
				continue
			}
			if err := scope.Adapter.Execute(txCtx, step.Seeder); err != nil {
				return fmt.Errorf("seeder %q: %w", step.Seeder.ID(), err)
			}
			if err := scope.History.MarkRun(txCtx, step.Seeder.ID()); err != nil {
				return err
			}
		}
		return nil
	})
}

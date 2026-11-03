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

func (r *Runner) Run(ctx context.Context, plan *ExecutionPlan) error {
	_, err := r.execute(ctx, plan)
	return err
}

// execute runs the plan and returns the IDs whose effects were actually
// persisted — on failure, only those that survived the rollback policy.
func (r *Runner) execute(ctx context.Context, plan *ExecutionPlan) ([]string, error) {
	switch r.txMode {
	case All:
		return r.runAll(ctx, plan)
	case PerSeeder:
		return r.runPerSeeder(ctx, plan)
	default:
		return r.runSequential(ctx, plan)
	}
}

func (r *Runner) runSequential(ctx context.Context, plan *ExecutionPlan) ([]string, error) {
	var applied []string
	for i, step := range plan.Steps {
		if step.Action == ActionSkip {
			continue
		}
		if err := r.adapter.Execute(ctx, step.Seeder); err != nil {
			return applied, &PartialRunError{FailedID: step.Seeder.ID(), Completed: i, Err: err}
		}
		if r.history != nil {
			if err := r.history.MarkRun(ctx, step.Seeder.ID()); err != nil {
				return applied, &PartialRunError{FailedID: step.Seeder.ID(), Completed: i, Err: err}
			}
		}
		applied = append(applied, step.Seeder.ID())
	}
	return applied, nil
}

func (r *Runner) runPerSeeder(ctx context.Context, plan *ExecutionPlan) ([]string, error) {
	txEngine, ok := r.adapter.(TransactionalEngine)
	if !ok {
		return nil, ErrTransactionNotSupported
	}

	var applied []string
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
			return applied, &PartialRunError{FailedID: step.Seeder.ID(), Completed: i, Err: err}
		}
		applied = append(applied, step.Seeder.ID())
	}
	return applied, nil
}

func (r *Runner) runAll(ctx context.Context, plan *ExecutionPlan) ([]string, error) {
	txEngine, ok := r.adapter.(TransactionalEngine)
	if !ok {
		return nil, ErrTransactionNotSupported
	}

	var applied []string
	err := txEngine.Transaction(ctx, func(txCtx context.Context, scope Scope) error {
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
			applied = append(applied, step.Seeder.ID())
		}
		return nil
	})
	if err != nil {
		return nil, err // whole batch rolled back: nothing was applied
	}
	return applied, nil
}

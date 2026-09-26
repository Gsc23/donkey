package core

import (
	"context"
	"errors"
	"testing"
)

type fakeAdapter struct {
	executed []string
	failOn   string
	failErr  error
}

func (a *fakeAdapter) Execute(ctx context.Context, s Identifiable) error {
	if s.ID() == a.failOn {
		return a.failErr
	}
	a.executed = append(a.executed, s.ID())
	return nil
}

func planFor(seeders ...Identifiable) *ExecutionPlan {
	steps := make([]Step, len(seeders))
	for i, s := range seeders {
		steps[i] = Step{Seeder: s, Action: ActionRun}
	}
	return &ExecutionPlan{Steps: steps}
}

func TestRunner_Sequential_Success(t *testing.T) {
	adapter := &fakeAdapter{}
	history := newFakeHistory()
	r := New(adapter, history, NoTransaction)

	plan := planFor(fakeSeeder{id: "a"}, fakeSeeder{id: "b", deps: []string{"a"}})
	if err := r.Run(context.Background(), plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := adapter.executed; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected execution order: %v", got)
	}
	if !history.ran["a"] || !history.ran["b"] {
		t.Fatalf("expected both seeders marked as run, got %v", history.ran)
	}
}

func TestRunner_Sequential_Failure_ReturnsPartialRunError(t *testing.T) {
	failErr := errors.New("boom")
	adapter := &fakeAdapter{failOn: "b", failErr: failErr}
	history := newFakeHistory()
	r := New(adapter, history, NoTransaction)

	plan := planFor(
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"b"}},
	)
	err := r.Run(context.Background(), plan)

	var partial *PartialRunError
	if !errors.As(err, &partial) {
		t.Fatalf("expected PartialRunError, got %v", err)
	}
	if partial.FailedID != "b" || partial.Completed != 1 {
		t.Fatalf("unexpected PartialRunError: %+v", partial)
	}
	if !errors.Is(err, failErr) {
		t.Fatalf("expected wrapped original error, got %v", err)
	}
	if got := adapter.executed; len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected only 'a' executed, got %v", got)
	}
	if history.ran["b"] || history.ran["c"] {
		t.Fatalf("did not expect 'b' or 'c' marked as run, got %v", history.ran)
	}
}

type fakeTxEngine struct {
	fakeAdapter
	history *fakeHistory
}

func (e *fakeTxEngine) Transaction(ctx context.Context, fn func(context.Context, Scope) error) error {
	executedBefore := append([]string{}, e.executed...)
	historyBefore := make(map[string]bool, len(e.history.ran))
	for k, v := range e.history.ran {
		historyBefore[k] = v
	}

	scope := Scope{Adapter: &e.fakeAdapter, History: e.history}
	if err := fn(ctx, scope); err != nil {
		e.executed = executedBefore
		e.history.ran = historyBefore
		return err
	}
	return nil
}

func TestRunner_PerSeeder_NotTransactional(t *testing.T) {
	adapter := &fakeAdapter{}
	r := New(adapter, newFakeHistory(), PerSeeder)

	err := r.Run(context.Background(), planFor(fakeSeeder{id: "a"}))
	if !errors.Is(err, ErrTransactionNotSupported) {
		t.Fatalf("expected ErrTransactionNotSupported, got %v", err)
	}
}

func TestRunner_PerSeeder_FailureIsIsolated(t *testing.T) {
	failErr := errors.New("boom")
	history := newFakeHistory()
	engine := &fakeTxEngine{
		fakeAdapter: fakeAdapter{failOn: "b", failErr: failErr},
		history:     history,
	}
	r := New(engine, history, PerSeeder)

	plan := planFor(
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"b"}},
	)
	err := r.Run(context.Background(), plan)

	var partial *PartialRunError
	if !errors.As(err, &partial) {
		t.Fatalf("expected PartialRunError, got %v", err)
	}
	if partial.FailedID != "b" || partial.Completed != 1 {
		t.Fatalf("unexpected PartialRunError: %+v", partial)
	}
	if got := engine.executed; len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected only 'a' committed, got %v", got)
	}
	if !history.ran["a"] {
		t.Fatal("expected 'a' preserved in history despite 'b' failing")
	}
	if history.ran["b"] || history.ran["c"] {
		t.Fatalf("did not expect 'b' or 'c' marked as run, got %v", history.ran)
	}
}

func TestRunner_All_FailureRollsBackEverything(t *testing.T) {
	failErr := errors.New("boom")
	history := newFakeHistory()
	engine := &fakeTxEngine{
		fakeAdapter: fakeAdapter{failOn: "b", failErr: failErr},
		history:     history,
	}
	r := New(engine, history, All)

	plan := planFor(
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"b"}},
	)
	err := r.Run(context.Background(), plan)

	var partial *PartialRunError
	if errors.As(err, &partial) {
		t.Fatalf("did not expect PartialRunError for All mode, got %+v", partial)
	}
	if !errors.Is(err, failErr) {
		t.Fatalf("expected wrapped original error, got %v", err)
	}
	if len(engine.executed) != 0 {
		t.Fatalf("expected full rollback, got executed=%v", engine.executed)
	}
	if len(history.ran) != 0 {
		t.Fatalf("expected full rollback of history, got %v", history.ran)
	}
}

func TestRunner_SkipsAlreadySkippedSteps(t *testing.T) {
	adapter := &fakeAdapter{}
	history := newFakeHistory()
	r := New(adapter, history, NoTransaction)

	plan := &ExecutionPlan{Steps: []Step{
		{Seeder: fakeSeeder{id: "a"}, Action: ActionSkip, Reason: "already executed"},
		{Seeder: fakeSeeder{id: "b", deps: []string{"a"}}, Action: ActionRun},
	}}
	if err := r.Run(context.Background(), plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := adapter.executed; len(got) != 1 || got[0] != "b" {
		t.Fatalf("expected only 'b' executed, got %v", got)
	}
}

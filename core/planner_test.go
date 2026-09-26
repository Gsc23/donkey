package core

import (
	"context"
	"errors"
	"testing"
)

type fakeHistory struct {
	ran map[string]bool
	err error
}

func newFakeHistory(ran ...string) *fakeHistory {
	m := make(map[string]bool, len(ran))
	for _, id := range ran {
		m[id] = true
	}
	return &fakeHistory{ran: m}
}

func (f *fakeHistory) EnsureSchema(ctx context.Context) error { return nil }

func (f *fakeHistory) HasRun(ctx context.Context, id string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.ran[id], nil
}

func (f *fakeHistory) MarkRun(ctx context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	if f.ran == nil {
		f.ran = make(map[string]bool)
	}
	f.ran[id] = true
	return nil
}

func TestPlanner_Compile_NoHistory_AllRun(t *testing.T) {
	p := NewPlanner(nil)
	seeders := []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
	}
	plan, err := p.Compile(context.Background(), seeders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(plan.Steps))
	}
	for _, step := range plan.Steps {
		if step.Action != ActionRun {
			t.Fatalf("expected ActionRun for %q, got %v", step.Seeder.ID(), step.Action)
		}
	}
	if plan.Steps[0].Seeder.ID() != "a" || plan.Steps[1].Seeder.ID() != "b" {
		t.Fatalf("unexpected order: [%s %s]", plan.Steps[0].Seeder.ID(), plan.Steps[1].Seeder.ID())
	}
}

func TestPlanner_Compile_SkipsAlreadyRun(t *testing.T) {
	history := newFakeHistory("a")
	p := NewPlanner(history)
	seeders := []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
	}
	plan, err := p.Compile(context.Background(), seeders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Steps[0].Action != ActionSkip || plan.Steps[0].Reason != "already executed" {
		t.Fatalf("expected step 'a' to be skipped, got %+v", plan.Steps[0])
	}
	if plan.Steps[1].Action != ActionRun {
		t.Fatalf("expected step 'b' to run, got %+v", plan.Steps[1])
	}
}

func TestPlanner_Compile_CycleError(t *testing.T) {
	p := NewPlanner(nil)
	seeders := []Identifiable{
		fakeSeeder{id: "a", deps: []string{"b"}},
		fakeSeeder{id: "b", deps: []string{"a"}},
	}
	_, err := p.Compile(context.Background(), seeders)
	var cycleErr *CycleError
	if !errors.As(err, &cycleErr) {
		t.Fatalf("expected CycleError, got %v", err)
	}
}

func TestPlanner_Compile_MissingDependency(t *testing.T) {
	p := NewPlanner(nil)
	seeders := []Identifiable{
		fakeSeeder{id: "a", deps: []string{"ghost"}},
	}
	_, err := p.Compile(context.Background(), seeders)
	if err == nil {
		t.Fatal("expected error for missing dependency")
	}
}

func TestPlanner_Compile_HistoryError(t *testing.T) {
	history := &fakeHistory{err: errors.New("boom")}
	p := NewPlanner(history)
	seeders := []Identifiable{fakeSeeder{id: "a"}}
	_, err := p.Compile(context.Background(), seeders)
	if err == nil {
		t.Fatal("expected error from history store")
	}
}

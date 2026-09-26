package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Gsc23/donkey/core"
	"github.com/Gsc23/donkey/core/cli"
)

type fakeSeeder struct {
	id   string
	deps []string
}

func (f fakeSeeder) ID() string             { return f.id }
func (f fakeSeeder) Dependencies() []string { return f.deps }

type fakeHistory struct {
	ran map[string]bool
}

func newFakeHistory(ran ...string) *fakeHistory {
	m := make(map[string]bool, len(ran))
	for _, id := range ran {
		m[id] = true
	}
	return &fakeHistory{ran: m}
}

func (h *fakeHistory) EnsureSchema(ctx context.Context) error { return nil }

func (h *fakeHistory) HasRun(ctx context.Context, id string) (bool, error) {
	return h.ran[id], nil
}

func (h *fakeHistory) MarkRun(ctx context.Context, id string) error {
	if h.ran == nil {
		h.ran = map[string]bool{}
	}
	h.ran[id] = true
	return nil
}

type fakeAdapter struct {
	failOn string
}

func (a *fakeAdapter) Execute(ctx context.Context, s core.Identifiable) error {
	if s.ID() == a.failOn {
		return errors.New("boom")
	}
	return nil
}

type fakeTxEngine struct {
	fakeAdapter
	history *fakeHistory
}

func (e *fakeTxEngine) Transaction(ctx context.Context, fn func(context.Context, core.Scope) error) error {
	scope := core.Scope{Adapter: &e.fakeAdapter, History: e.history}
	return fn(ctx, scope)
}

func runCommand(t *testing.T, runner *core.Runner, seeders []core.Identifiable, args ...string) (string, error) {
	t.Helper()
	cmd := cli.New(runner, seeders)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard) // cobra's own "Error: ..." line isn't what we're asserting on here
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestRunCmd_PrintsSeededLineOnSuccess(t *testing.T) {
	history := newFakeHistory()
	runner := core.New(&fakeAdapter{}, history, core.NoTransaction)
	seeders := []core.Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
	}

	out, err := runCommand(t, runner, seeders, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "seeded: a\nseeded: b\n"
	if out != want {
		t.Fatalf("esperava %q, got %q", want, out)
	}
}

func TestRunCmd_SkippedStepsDoNotPrintSeededLine(t *testing.T) {
	history := newFakeHistory("a")
	runner := core.New(&fakeAdapter{}, history, core.NoTransaction)
	seeders := []core.Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
	}

	out, err := runCommand(t, runner, seeders, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "seeded: a") {
		t.Fatalf("não esperava linha pra 'a' (já executado antes desta run), got %q", out)
	}
	if !strings.Contains(out, "seeded: b") {
		t.Fatalf("esperava linha pra 'b', got %q", out)
	}
}

func TestRunCmd_DryRun_PrintsNoSeededLines(t *testing.T) {
	history := newFakeHistory()
	runner := core.New(&fakeAdapter{}, history, core.NoTransaction)
	seeders := []core.Identifiable{fakeSeeder{id: "a"}}

	out, err := runCommand(t, runner, seeders, "run", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "seeded:") {
		t.Fatalf("dry-run não deveria imprimir 'seeded:', got %q", out)
	}
	if history.ran["a"] {
		t.Fatal("dry-run não deveria ter marcado histórico")
	}
}

func TestRunCmd_PerSeeder_PartialFailure_PrintsOnlyCompletedSteps(t *testing.T) {
	history := newFakeHistory()
	engine := &fakeTxEngine{fakeAdapter: fakeAdapter{failOn: "b"}, history: history}
	runner := core.New(engine, history, core.PerSeeder)
	seeders := []core.Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"b"}},
	}

	out, err := runCommand(t, runner, seeders, "run")
	var partial *core.PartialRunError
	if !errors.As(err, &partial) {
		t.Fatalf("esperava PartialRunError, got %v", err)
	}
	if out != "seeded: a\n" {
		t.Fatalf("esperava só 'seeded: a', got %q", out)
	}
}

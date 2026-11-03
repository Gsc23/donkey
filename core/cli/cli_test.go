package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/Gsc23/donkey/core"
	"github.com/Gsc23/donkey/core/cli"
)

// fakeSeeds stands in for the driving port: CLI tests only check flag
// parsing and output formatting. Orchestration is tested in core.
type fakeSeeds struct {
	order   []string
	status  []core.StepStatus
	report  core.RunReport
	err     error
	gotOpts core.RunOptions
	newPath string
	gotNew  core.NewSeederRequest
}

func (f *fakeSeeds) Order(context.Context) ([]string, error)           { return f.order, f.err }
func (f *fakeSeeds) Status(context.Context) ([]core.StepStatus, error) { return f.status, f.err }
func (f *fakeSeeds) Run(_ context.Context, opt core.RunOptions) (core.RunReport, error) {
	f.gotOpts = opt
	return f.report, f.err
}

func (f *fakeSeeds) New(_ context.Context, req core.NewSeederRequest) (string, error) {
	f.gotNew = req
	return f.newPath, f.err
}

func runCommand(t *testing.T, svc cli.Seeds, args ...string) (string, error) {
	t.Helper()
	cmd := cli.New(svc)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard) // cobra's own "Error: ..." line isn't what we're asserting on here
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestListCmd_PrintsOrder(t *testing.T) {
	out, err := runCommand(t, &fakeSeeds{order: []string{"a", "b"}}, "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "a\nb\n"; out != want {
		t.Fatalf("esperava %q, got %q", want, out)
	}
}

func TestPlanCmd_PrintsNumbered(t *testing.T) {
	out, err := runCommand(t, &fakeSeeds{order: []string{"a", "b"}}, "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "1. a\n2. b\n"; out != want {
		t.Fatalf("esperava %q, got %q", want, out)
	}
}

func TestPlanCmd_CycleError_ReturnsErrorWithoutExtraOutput(t *testing.T) {
	cycle := &core.CycleError{Path: []string{"a", "b", "a"}}
	out, err := runCommand(t, &fakeSeeds{err: cycle}, "plan")
	if !errors.Is(err, cycle) {
		t.Fatalf("esperava CycleError, got %v", err)
	}
	if out != "" {
		t.Fatalf("esperava stdout vazio, got %q", out)
	}
}

func TestRunCmd_NothingPending_SaysSo(t *testing.T) {
	for _, args := range [][]string{{"run"}, {"run", "--dry-run"}} {
		out, err := runCommand(t, &fakeSeeds{}, args...)
		if err != nil {
			t.Fatalf("%v: unexpected error: %v", args, err)
		}
		if want := "nothing pending — all seeders already applied\n"; out != want {
			t.Fatalf("%v: esperava %q, got %q", args, want, out)
		}
	}
}

func TestRunCmd_ErrorWithNothingPending_DoesNotClaimSuccess(t *testing.T) {
	cycle := &core.CycleError{Path: []string{"a", "b", "a"}}
	out, err := runCommand(t, &fakeSeeds{err: cycle}, "run")
	if !errors.Is(err, cycle) {
		t.Fatalf("esperava CycleError, got %v", err)
	}
	if out != "" {
		t.Fatalf("esperava stdout vazio, got %q", out)
	}
}

func TestRootHelp_EveryCommandHasShortDescription(t *testing.T) {
	for _, c := range cli.New(&fakeSeeds{}).Commands() {
		if c.Short == "" {
			t.Errorf("comando %q sem Short — aparece em branco no --help", c.Name())
		}
	}
}

func TestStatusCmd_MarksAppliedAndPending(t *testing.T) {
	svc := &fakeSeeds{status: []core.StepStatus{{ID: "a", Applied: true}, {ID: "b"}}}
	out, err := runCommand(t, svc, "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "a                                        ✓\nb                                        ✗\n"
	if out != want {
		t.Fatalf("esperava %q, got %q", want, out)
	}
}

func TestRunCmd_PrintsSeededLinePerAppliedStep(t *testing.T) {
	svc := &fakeSeeds{report: core.RunReport{Pending: []string{"a", "b"}, Applied: []string{"a", "b"}}}
	out, err := runCommand(t, svc, "run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.gotOpts.DryRun {
		t.Fatal("run sem --dry-run não deveria pedir DryRun")
	}
	if want := "seeded: a\nseeded: b\n"; out != want {
		t.Fatalf("esperava %q, got %q", want, out)
	}
}

func TestRunCmd_DryRun_PrintsPendingWithoutSeededLines(t *testing.T) {
	svc := &fakeSeeds{report: core.RunReport{Pending: []string{"b"}}}
	out, err := runCommand(t, svc, "run", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !svc.gotOpts.DryRun {
		t.Fatal("--dry-run deveria chegar como RunOptions.DryRun")
	}
	if want := "[dry-run] would run: b\n"; out != want {
		t.Fatalf("esperava %q, got %q", want, out)
	}
}

func TestRunCmd_PartialFailure_PrintsAppliedThenReturnsError(t *testing.T) {
	partial := &core.PartialRunError{FailedID: "b", Completed: 1, Err: errors.New("boom")}
	svc := &fakeSeeds{report: core.RunReport{Pending: []string{"a", "b", "c"}, Applied: []string{"a"}}, err: partial}

	out, err := runCommand(t, svc, "run")
	if !errors.Is(err, partial) {
		t.Fatalf("esperava PartialRunError, got %v", err)
	}
	if out != "seeded: a\n" {
		t.Fatalf("esperava só 'seeded: a', got %q", out)
	}
}

func TestNewCmd_PassesFlagsToServiceAndPrintsPath(t *testing.T) {
	svc := &fakeSeeds{newPath: "db/seeds/20261102_create-orders.go"}
	out, err := runCommand(t, svc, "new", "create-orders",
		"--dir", "db/seeds", "--package", "seeds", "--deps", "x,y")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := core.NewSeederRequest{Slug: "create-orders", Dir: "db/seeds", Package: "seeds", Deps: []string{"x", "y"}}
	if !reflect.DeepEqual(svc.gotNew, want) {
		t.Fatalf("esperava request %+v, got %+v", want, svc.gotNew)
	}
	if out != svc.newPath+"\n" {
		t.Fatalf("esperava imprimir o caminho, got %q", out)
	}
}

func TestNewCmd_DefaultDir(t *testing.T) {
	svc := &fakeSeeds{}
	if _, err := runCommand(t, svc, "new", "create-orders"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.gotNew.Dir != "./seeders" || svc.gotNew.Package != "" || svc.gotNew.Deps != nil {
		t.Fatalf("defaults inesperados: %+v", svc.gotNew)
	}
}

func TestNewCmd_RequiresExactlyOneName(t *testing.T) {
	if _, err := runCommand(t, &fakeSeeds{}, "new"); err == nil {
		t.Fatal("esperava erro sem <nome>")
	}
}

package core

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// StepStatus is one seeder's position in the plan and whether its effects
// are already recorded in history.
type StepStatus struct {
	ID      string
	Applied bool
}

type RunOptions struct {
	// DryRun compiles the plan against the real history but executes nothing
	// and writes nothing — RunReport.Pending says what a real run would do.
	DryRun bool
}

type RunReport struct {
	// Pending lists, in execution order, the seeders the plan scheduled to
	// run (i.e. not yet in history when the run started).
	Pending []string
	// Applied lists the seeders whose effects were persisted by this run.
	// On a PartialRunError it holds only those committed before the failure;
	// under All it is empty on any failure, since the batch rolled back.
	Applied []string
}

// Service is the driving port of core: the use cases a frontend (the CLI,
// or anything else) invokes. Planner and Runner are mechanism behind it.
type Service struct {
	runner     *Runner
	seeders    []Identifiable
	scaffolder Scaffolder
	now        func() time.Time
}

type ServiceOption func(*Service)

// WithScaffolder enables Service.New (the `seed new` command).
func WithScaffolder(s Scaffolder) ServiceOption {
	return func(svc *Service) { svc.scaffolder = s }
}

func NewService(runner *Runner, seeders []Identifiable, opts ...ServiceOption) *Service {
	svc := &Service{runner: runner, seeders: seeders, now: time.Now}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// Order returns every seeder ID in dependency order. It ignores history and
// never touches the database.
func (s *Service) Order(ctx context.Context) ([]string, error) {
	plan, err := NewPlanner(nil).Compile(ctx, s.seeders)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(plan.Steps))
	for i, step := range plan.Steps {
		ids[i] = step.Seeder.ID()
	}
	return ids, nil
}

// Status returns every seeder in dependency order, flagged with whether
// history already records it. Read-only.
func (s *Service) Status(ctx context.Context) ([]StepStatus, error) {
	plan, err := NewPlanner(s.runner.history).Compile(ctx, s.seeders)
	if err != nil {
		return nil, err
	}
	out := make([]StepStatus, len(plan.Steps))
	for i, step := range plan.Steps {
		out[i] = StepStatus{ID: step.Seeder.ID(), Applied: step.Action == ActionSkip}
	}
	return out, nil
}

// Run applies every pending seeder. The report is meaningful even when err
// is non-nil: Applied tells what was persisted before the failure.
func (s *Service) Run(ctx context.Context, opt RunOptions) (RunReport, error) {
	plan, err := NewPlanner(s.runner.history).Compile(ctx, s.seeders)
	if err != nil {
		return RunReport{}, err
	}

	var report RunReport
	for _, step := range plan.Steps {
		if step.Action == ActionRun {
			report.Pending = append(report.Pending, step.Seeder.ID())
		}
	}
	if opt.DryRun {
		return report, nil
	}

	report.Applied, err = s.runner.execute(ctx, plan)
	return report, err
}

// slugPattern: lowercase words joined by hyphens, starting with a letter so
// the derived Go type name (create-orders → CreateOrders) is an identifier.
var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// New creates the source file of a new seeder. Core enforces the rules it
// owns — ID format, dependencies that exist, no ID collision — before the
// Scaffolder writes anything, so a bad --deps fails now instead of at the
// next plan.
func (s *Service) New(ctx context.Context, req NewSeederRequest) (string, error) {
	if s.scaffolder == nil {
		return "", ErrScaffolderNotConfigured
	}
	if !slugPattern.MatchString(req.Slug) {
		return "", fmt.Errorf("invalid name %q: use lowercase letters, digits and hyphens, starting with a letter (e.g. create-orders)", req.Slug)
	}

	now := s.now()
	id := now.Format("2006-01-02") + "-" + req.Slug

	known := make(map[string]bool, len(s.seeders))
	for _, seeder := range s.seeders {
		known[seeder.ID()] = true
	}
	if known[id] {
		return "", fmt.Errorf("a seeder with ID %q is already registered", id)
	}
	var unknown []string
	for _, dep := range req.Deps {
		if !known[dep] {
			unknown = append(unknown, dep)
		}
	}
	if len(unknown) > 0 {
		return "", fmt.Errorf("--deps references unregistered seeder(s): %s", strings.Join(unknown, ", "))
	}

	return s.scaffolder.Scaffold(ctx, SeederSpec{
		ID:        id,
		Slug:      req.Slug,
		Deps:      req.Deps,
		CreatedAt: now,
		Dir:       req.Dir,
		Package:   req.Package,
	})
}

package core

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestService_Order_IgnoresHistory(t *testing.T) {
	svc := NewService(New(&fakeAdapter{}, newFakeHistory("a"), NoTransaction), []Identifiable{
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "a"},
	})

	got, err := svc.Order(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestService_Status_FlagsApplied(t *testing.T) {
	svc := NewService(New(&fakeAdapter{}, newFakeHistory("a"), NoTransaction), []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
	})

	got, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []StepStatus{{ID: "a", Applied: true}, {ID: "b", Applied: false}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestService_Run_ReportsAppliedSkippingHistory(t *testing.T) {
	history := newFakeHistory("a")
	svc := NewService(New(&fakeAdapter{}, history, NoTransaction), []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
	})

	report, err := svc.Run(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(report.Pending, []string{"b"}) || !reflect.DeepEqual(report.Applied, []string{"b"}) {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestService_Run_DryRun_ExecutesAndWritesNothing(t *testing.T) {
	adapter := &fakeAdapter{}
	history := newFakeHistory("a")
	svc := NewService(New(adapter, history, NoTransaction), []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
	})

	report, err := svc.Run(context.Background(), RunOptions{DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(report.Pending, []string{"b"}) {
		t.Fatalf("dry-run deveria reportar só o pendente 'b', got %v", report.Pending)
	}
	if len(report.Applied) != 0 || len(adapter.executed) != 0 || history.ran["b"] {
		t.Fatalf("dry-run não deveria executar nem marcar nada: report=%+v executed=%v", report, adapter.executed)
	}
}

func TestService_Run_PerSeeder_PartialFailure_ReportsCommittedOnly(t *testing.T) {
	history := newFakeHistory()
	engine := &fakeTxEngine{fakeAdapter: fakeAdapter{failOn: "b", failErr: errors.New("boom")}, history: history}
	svc := NewService(New(engine, history, PerSeeder), []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"b"}},
	})

	report, err := svc.Run(context.Background(), RunOptions{})
	var partial *PartialRunError
	if !errors.As(err, &partial) {
		t.Fatalf("expected PartialRunError, got %v", err)
	}
	if !reflect.DeepEqual(report.Applied, []string{"a"}) {
		t.Fatalf("esperava só 'a' aplicado, got %v", report.Applied)
	}
}

func TestService_Run_All_FailureReportsNothingApplied(t *testing.T) {
	history := newFakeHistory()
	engine := &fakeTxEngine{fakeAdapter: fakeAdapter{failOn: "b", failErr: errors.New("boom")}, history: history}
	svc := NewService(New(engine, history, All), []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
	})

	report, err := svc.Run(context.Background(), RunOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(report.Applied) != 0 {
		t.Fatalf("All faz rollback do lote inteiro — nada aplicado, got %v", report.Applied)
	}
}

func TestService_Run_CycleError(t *testing.T) {
	svc := NewService(New(&fakeAdapter{}, nil, NoTransaction), []Identifiable{
		fakeSeeder{id: "a", deps: []string{"b"}},
		fakeSeeder{id: "b", deps: []string{"a"}},
	})

	_, err := svc.Run(context.Background(), RunOptions{})
	var cycle *CycleError
	if !errors.As(err, &cycle) {
		t.Fatalf("expected CycleError, got %v", err)
	}
}

type fakeScaffolder struct {
	got  *SeederSpec
	path string
}

func (f *fakeScaffolder) Scaffold(_ context.Context, spec SeederSpec) (string, error) {
	f.got = &spec
	return f.path, nil
}

func newServiceForScaffold(sc Scaffolder, seeders ...Identifiable) *Service {
	svc := NewService(New(&fakeAdapter{}, nil, NoTransaction), seeders, WithScaffolder(sc))
	svc.now = func() time.Time { return time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC) }
	return svc
}

func TestService_New_BuildsSpecAndDelegates(t *testing.T) {
	sc := &fakeScaffolder{path: "seeders/20261102_create-orders.go"}
	svc := newServiceForScaffold(sc, fakeSeeder{id: "2026-01-01-create-admin"})

	path, err := svc.New(context.Background(), NewSeederRequest{
		Slug: "create-orders", Deps: []string{"2026-01-01-create-admin"}, Dir: "seeders", Package: "seeds",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != sc.path {
		t.Fatalf("esperava o caminho do Scaffolder, got %q", path)
	}
	want := SeederSpec{
		ID: "2026-11-02-create-orders", Slug: "create-orders", Deps: []string{"2026-01-01-create-admin"},
		CreatedAt: svc.now(), Dir: "seeders", Package: "seeds",
	}
	if !reflect.DeepEqual(*sc.got, want) {
		t.Fatalf("spec inesperada:\nwant %+v\ngot  %+v", want, *sc.got)
	}
}

func TestService_New_RejectsBeforeScaffolding(t *testing.T) {
	registered := fakeSeeder{id: "2026-01-01-create-admin"}
	cases := map[string]NewSeederRequest{
		"slug com maiúscula":        {Slug: "CreateOrders"},
		"slug com espaço":           {Slug: "create orders"},
		"slug com underscore":       {Slug: "create_orders"},
		"slug começando por dígito": {Slug: "1-orders"}, // viraria type 1Orders
		"dep não registrada":        {Slug: "create-orders", Deps: []string{"2026-01-01-create-admin", "nao-existe"}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			sc := &fakeScaffolder{}
			if _, err := newServiceForScaffold(sc, registered).New(context.Background(), req); err == nil {
				t.Fatal("esperava erro")
			}
			if sc.got != nil {
				t.Fatal("Scaffolder não deveria ter sido chamado")
			}
		})
	}
}

func TestService_New_RejectsIDCollision(t *testing.T) {
	sc := &fakeScaffolder{}
	svc := newServiceForScaffold(sc, fakeSeeder{id: "2026-11-02-create-orders"})
	if _, err := svc.New(context.Background(), NewSeederRequest{Slug: "create-orders"}); err == nil {
		t.Fatal("esperava erro de ID já registrado")
	}
	if sc.got != nil {
		t.Fatal("Scaffolder não deveria ter sido chamado")
	}
}

func TestService_New_WithoutScaffolder(t *testing.T) {
	svc := NewService(New(&fakeAdapter{}, nil, NoTransaction), nil)
	_, err := svc.New(context.Background(), NewSeederRequest{Slug: "create-orders"})
	if !errors.Is(err, ErrScaffolderNotConfigured) {
		t.Fatalf("esperava ErrScaffolderNotConfigured, got %v", err)
	}
}

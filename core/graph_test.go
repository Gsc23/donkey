package core

import (
	"reflect"
	"testing"
)

type fakeSeeder struct {
	id   string
	deps []string
}

func (f fakeSeeder) ID() string             { return f.id }
func (f fakeSeeder) Dependencies() []string { return f.deps }

func TestBuildGraph_MissingDependency(t *testing.T) {
	seeders := []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"ghost"}},
	}
	_, err := buildGraph(seeders)
	if err == nil {
		t.Fatal("esperava erro para dependência inexistente")
	}
}

func TestDetectCycle_WithoutCycle(t *testing.T) {
	seeders := []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
	}
	g, err := buildGraph(seeders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle := g.DetectCycle(); cycle != nil {
		t.Fatalf("não esperava ciclo, obteve %v", cycle)
	}
}

func TestDetectCycle_WithCycle(t *testing.T) {
	seeders := []Identifiable{
		fakeSeeder{id: "a", deps: []string{"c"}},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"b"}},
	}
	g, err := buildGraph(seeders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cycle := g.DetectCycle()
	want := []string{"a", "c", "b", "a"}
	if !reflect.DeepEqual(cycle, want) {
		t.Fatalf("esperava ciclo %v, obteve %v", want, cycle)
	}
}

func TestDetectCycle_IndirectCycle(t *testing.T) {
	seeders := []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a", "d"}},
		fakeSeeder{id: "c", deps: []string{"b"}},
		fakeSeeder{id: "d", deps: []string{"c"}},
	}
	g, err := buildGraph(seeders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycle := g.DetectCycle(); cycle == nil {
		t.Fatal("esperava detectar ciclo indireto b->d->c->b")
	}
}

func TestTopoSort_fail(t *testing.T) {
	seeders := []Identifiable{
		fakeSeeder{id: "a", deps: []string{"c"}},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"b"}},
	}
	g, err := buildGraph(seeders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = g.TopoSort()
	if err == nil {
		t.Fatal("expected cycle to fail topological sort")
	}
}

func TestTopoSort_Unordered(t *testing.T) {
	seeders := []Identifiable{
		fakeSeeder{id: "c", deps: []string{"b"}},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "a"},
	}
	g, err := buildGraph(seeders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ordered, err := g.TopoSort()
	if err != nil {
		t.Fatalf("unexpected error to topological sort: %v", err)
	}
	if got := ids(ordered); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("esperava ordem [a b c], obteve %v", got)
	}
}

func TestTopoSort_Ordered(t *testing.T) {
	seeders := []Identifiable{
		fakeSeeder{id: "a"},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"b"}},
	}
	g, err := buildGraph(seeders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ordered, err := g.TopoSort()
	if err != nil {
		t.Fatalf("unexpected error to topological sort: %v", err)
	}
	if got := ids(ordered); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("esperava ordem [a b c], obteve %v", got)
	}
}

func TestTopoSort_DiamondDependency(t *testing.T) {
	// a <- b, a <- c, b,c <- d  (d depends on both b and c, which both depend on a)
	seeders := []Identifiable{
		fakeSeeder{id: "d", deps: []string{"b", "c"}},
		fakeSeeder{id: "b", deps: []string{"a"}},
		fakeSeeder{id: "c", deps: []string{"a"}},
		fakeSeeder{id: "a"},
	}
	g, err := buildGraph(seeders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ordered, err := g.TopoSort()
	if err != nil {
		t.Fatalf("unexpected error to topological sort: %v", err)
	}
	got := ids(ordered)
	pos := make(map[string]int, len(got))
	for i, id := range got {
		pos[id] = i
	}
	if pos["a"] > pos["b"] || pos["a"] > pos["c"] || pos["b"] > pos["d"] || pos["c"] > pos["d"] {
		t.Fatalf("ordem topológica inválida: %v", got)
	}
}

func ids(seeders []Identifiable) []string {
	out := make([]string, len(seeders))
	for i, s := range seeders {
		out[i] = s.ID()
	}
	return out
}

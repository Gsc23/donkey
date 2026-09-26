package core

import (
	"fmt"
	"sort"
)

type nodeColor int

const (
	white nodeColor = iota
	gray
	black
)

type graph struct {
	nodes map[string]Identifiable
	edges map[string][]string
}

func buildGraph(seeders []Identifiable) (*graph, error) {
	g := &graph{
		nodes: make(map[string]Identifiable, len(seeders)),
		edges: make(map[string][]string, len(seeders)),
	}
	for _, s := range seeders {
		g.nodes[s.ID()] = s
		g.edges[s.ID()] = s.Dependencies()
	}
	for id, deps := range g.edges {
		for _, dep := range deps {
			if _, ok := g.nodes[dep]; !ok {
				return nil, fmt.Errorf("seeder %q depende de seeder inexistente %q", id, dep)
			}
		}
	}
	return g, nil
}

func (g *graph) DetectCycle() []string {
	colors := make(map[string]nodeColor, len(g.nodes))
	var path []string
	var cycle []string

	var visit func(id string)
	visit = func(id string) {
		colors[id] = gray
		path = append(path, id)
		for _, dep := range g.edges[id] {
			if cycle != nil {
				return
			}
			switch colors[dep] {
			case white:
				visit(dep)
			case gray:
				start := indexOf(path, dep)
				cycle = append(append([]string{}, path[start:]...), dep)
			}
		}
		if cycle == nil {
			path = path[:len(path)-1]
			colors[id] = black
		}
	}

	for _, id := range sortedIDs(g.nodes) {
		if colors[id] == white {
			visit(id)
		}
		if cycle != nil {
			return cycle
		}
	}
	return nil
}

func (g *graph) TopoSort() ([]Identifiable, error) {
	if cycle := g.DetectCycle(); cycle != nil {
		return nil, &CycleError{Path: cycle}
	}

	colors := make(map[string]nodeColor, len(g.nodes))
	order := make([]Identifiable, 0, len(g.nodes))

	var visit func(id string)
	visit = func(id string) {
		colors[id] = gray
		for _, dep := range g.edges[id] {
			if colors[dep] == white {
				visit(dep)
			}
		}
		colors[id] = black
		order = append(order, g.nodes[id])
	}

	for _, id := range sortedIDs(g.nodes) {
		if colors[id] == white {
			visit(id)
		}
	}
	return order, nil
}

func sortedIDs(nodes map[string]Identifiable) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

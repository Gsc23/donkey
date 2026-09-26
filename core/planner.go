package core

import "context"

type Planner struct {
	history HistoryStore
}

func NewPlanner(h HistoryStore) *Planner {
	return &Planner{history: h}
}

func (p *Planner) Compile(ctx context.Context, seeders []Identifiable) (*ExecutionPlan, error) {
	g, err := buildGraph(seeders)
	if err != nil {
		return nil, err
	}
	ordered, err := g.TopoSort()
	if err != nil {
		return nil, err
	}

	steps := make([]Step, 0, len(ordered))
	for _, s := range ordered {
		step := Step{Seeder: s, Action: ActionRun}
		if p.history != nil {
			ran, err := p.history.HasRun(ctx, s.ID())
			if err != nil {
				return nil, err
			}
			if ran {
				step.Action = ActionSkip
				step.Reason = "already executed"
			}
		}
		steps = append(steps, step)
	}
	return &ExecutionPlan{Steps: steps}, nil
}

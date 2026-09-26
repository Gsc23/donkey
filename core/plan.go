package core

type StepAction int

const (
	ActionRun StepAction = iota
	ActionSkip
)

type Step struct {
	Seeder Identifiable
	Action StepAction
	Reason string
}

type ExecutionPlan struct {
	Steps []Step
}

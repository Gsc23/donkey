package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Gsc23/donkey/adapter/noop"
	"github.com/Gsc23/donkey/core"
)

func New(runner *core.Runner, seeders []core.Identifiable) *cobra.Command {
	root := &cobra.Command{Use: "seed"}
	root.AddCommand(listCmd(seeders))
	root.AddCommand(planCmd(seeders))
	root.AddCommand(runCmd(runner, seeders))
	root.AddCommand(statusCmd(runner, seeders))
	return root
}

func listCmd(seeders []core.Identifiable) *cobra.Command {
	return &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := core.NewPlanner(nil).Compile(cmd.Context(), seeders)
			if err != nil {
				return err
			}
			for _, s := range plan.Steps {
				fmt.Println(s.Seeder.ID())
			}
			return nil
		},
	}
}

func planCmd(seeders []core.Identifiable) *cobra.Command {
	return &cobra.Command{
		Use: "plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := core.NewPlanner(nil).Compile(cmd.Context(), seeders)
			if err != nil {
				var cycleErr *core.CycleError
				if errors.As(err, &cycleErr) {
					fmt.Println("circular dependency:", cycleErr.Path)
				}
				return err
			}
			for i, s := range plan.Steps {
				fmt.Printf("%d. %s\n", i+1, s.Seeder.ID())
			}
			return nil
		},
	}
}

func runCmd(runner *core.Runner, seeders []core.Identifiable) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use: "run",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := core.NewPlanner(runner.History()).Compile(cmd.Context(), seeders)
			if err != nil {
				return err
			}

			r := runner
			if dryRun {
				r = core.New(noop.New(), nil, core.NoTransaction)
			}
			return r.Run(cmd.Context(), plan)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "não executa de fato, só mostra o que rodaria")
	return cmd
}

func statusCmd(runner *core.Runner, seeders []core.Identifiable) *cobra.Command {
	return &cobra.Command{
		Use: "status",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := core.NewPlanner(runner.History()).Compile(cmd.Context(), seeders)
			if err != nil {
				return err
			}
			for _, s := range plan.Steps {
				mark := "✗"
				if s.Action == core.ActionSkip {
					mark = "✓"
				}
				fmt.Printf("%-40s %s\n", s.Seeder.ID(), mark)
			}
			return nil
		},
	}
}

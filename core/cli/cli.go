package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Gsc23/donkey/core"
)

// Seeds is the driving port this CLI consumes. *core.Service implements it;
// the CLI only parses flags and formats output — it never sees Planner,
// Runner or ExecutionPlan.
type Seeds interface {
	Order(ctx context.Context) ([]string, error)
	Status(ctx context.Context) ([]core.StepStatus, error)
	Run(ctx context.Context, opt core.RunOptions) (core.RunReport, error)
	New(ctx context.Context, req core.NewSeederRequest) (path string, err error)
}

func New(svc Seeds, extra ...*cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "seed"}
	root.SilenceUsage = true
	root.AddCommand(listCmd(svc))
	root.AddCommand(planCmd(svc))
	root.AddCommand(runCmd(svc))
	root.AddCommand(statusCmd(svc))
	root.AddCommand(newCmd(svc))
	for _, cmd := range extra {
		root.AddCommand(cmd)
	}
	return root
}

func listCmd(svc Seeds) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List seeders in execution order",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := svc.Order(cmd.Context())
			if err != nil {
				return err
			}
			for _, id := range ids {
				fmt.Fprintln(cmd.OutOrStdout(), id)
			}
			return nil
		},
	}
}

func planCmd(svc Seeds) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Show the numbered execution plan (resolves dependencies, detects cycles)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := svc.Order(cmd.Context())
			if err != nil {
				return err
			}
			for i, id := range ids {
				fmt.Fprintf(cmd.OutOrStdout(), "%d. %s\n", i+1, id)
			}
			return nil
		},
	}
}

func runCmd(svc Seeds) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run pending seeders, skipping the ones already applied",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := svc.Run(cmd.Context(), core.RunOptions{DryRun: dryRun})
			out := cmd.OutOrStdout()
			if err == nil && len(report.Pending) == 0 {
				fmt.Fprintln(out, "nothing pending — all seeders already applied")
				return nil
			}
			if dryRun {
				for _, id := range report.Pending {
					fmt.Fprintf(out, "[dry-run] would run: %s\n", id)
				}
				return err
			}
			for _, id := range report.Applied {
				fmt.Fprintf(out, "seeded: %s\n", id)
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would run without executing anything")
	return cmd
}

func statusCmd(svc Seeds) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which seeders are applied (✓) and which are pending (✗)",
		RunE: func(cmd *cobra.Command, args []string) error {
			steps, err := svc.Status(cmd.Context())
			if err != nil {
				return err
			}
			for _, s := range steps {
				mark := "✗"
				if s.Applied {
					mark = "✓"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-40s %s\n", s.ID, mark)
			}
			return nil
		},
	}
}

func newCmd(svc Seeds) *cobra.Command {
	var req core.NewSeederRequest
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Generate a new seeder file from the configured adapter's template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Slug = args[0]
			path, err := svc.New(cmd.Context(), req)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	cmd.Flags().StringVar(&req.Dir, "dir", "./seeders", "directory where the seeder file is created")
	cmd.Flags().StringVar(&req.Package, "package", "", "Go package of the generated file (default: basename of --dir)")
	cmd.Flags().StringSliceVar(&req.Deps, "deps", nil, "IDs of already-registered seeders this one depends on")
	return cmd
}

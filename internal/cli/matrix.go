package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime/docker"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"github.com/spf13/cobra"
)

// newLazyMatrixCmd creates the matrix command group that initializes
// the Docker runtime and controller only when a subcommand is invoked.
func newLazyMatrixCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "matrix",
		Aliases: []string{"mx"},
		Short:   "Manage coordinated multi-sandbox matrices",
	}

	var ctrl *controller.Controller

	initController := func() error {
		if ctrl != nil {
			return nil
		}
		rt, err := docker.New()
		if err != nil {
			return fmt.Errorf("initialize Docker runtime: %w\n\nIs Docker running?", err)
		}
		store, err := state.NewFileStore()
		if err != nil {
			return fmt.Errorf("initialize state store: %w", err)
		}
		sessions, err := state.NewFileSessionStore()
		if err != nil {
			return fmt.Errorf("initialize session store: %w", err)
		}
		matrices, err := state.NewFileMatrixStore()
		if err != nil {
			return fmt.Errorf("initialize matrix store: %w", err)
		}
		ctrl = controller.New(rt, store, sessions, matrices)
		if err := ctrl.Reconcile(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: state reconciliation failed: %v\n", err)
		}
		return nil
	}

	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		return initController()
	}

	cmd.AddCommand(
		newMatrixCreateCmdLazy(&ctrl),
		newMatrixListCmdLazy(&ctrl),
		newMatrixInspectCmdLazy(&ctrl),
		newMatrixStopCmdLazy(&ctrl),
		newMatrixStartCmdLazy(&ctrl),
		newMatrixDestroyCmdLazy(&ctrl),
	)

	return cmd
}

func newMatrixCreateCmdLazy(ctrl **controller.Controller) *cobra.Command {
	var members []string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new matrix of coordinated sandboxes",
		Long: `Create a new matrix with multiple member sandboxes.

Each member is specified as name:blueprint, for example:
  smx matrix create fullstack --member frontend:blueprints/node-dev.yaml --member backend:blueprints/python-dev.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			if len(members) == 0 {
				return fmt.Errorf("at least one --member is required")
			}

			parsed, err := parseMembers(members)
			if err != nil {
				return err
			}

			fmt.Printf("Creating matrix %q with %d members...\n", name, len(parsed))
			mx, err := (*ctrl).CreateMatrix(context.Background(), name, parsed)
			if err != nil {
				return err
			}
			fmt.Printf("Matrix %q is %s.\n", mx.Metadata.Name, mx.State)
			for _, m := range mx.Members {
				fmt.Printf("  - %s (%s)\n", m.Name, m.Blueprint)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVarP(&members, "member", "m", nil, "Member in name:blueprint format (repeatable)")
	return cmd
}

func newMatrixListCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all matrices",
		RunE: func(_ *cobra.Command, _ []string) error {
			matrices, err := (*ctrl).ListMatrices()
			if err != nil {
				return err
			}
			if len(matrices) == 0 {
				fmt.Println("No matrices found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATE\tMEMBERS\tAGE")
			for _, mx := range matrices {
				age := time.Since(mx.Metadata.CreatedAt).Truncate(time.Second)
				memberNames := make([]string, len(mx.Members))
				for i, m := range mx.Members {
					memberNames[i] = m.Name
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					mx.Metadata.Name,
					mx.State,
					strings.Join(memberNames, ","),
					age,
				)
			}
			return w.Flush()
		},
	}
}

func newMatrixInspectCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <name>",
		Short: "Show detailed matrix information",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			mx, err := (*ctrl).GetMatrix(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Name:     %s\n", mx.Metadata.Name)
			fmt.Printf("State:    %s\n", mx.State)
			fmt.Printf("Created:  %s\n", mx.Metadata.CreatedAt.Format(time.RFC3339))
			fmt.Printf("Members:\n")
			for _, m := range mx.Members {
				sandboxName := mx.Metadata.Name + "-" + m.Name
				fmt.Printf("  - %s (blueprint: %s, sandbox: %s)\n", m.Name, m.Blueprint, sandboxName)
			}
			return nil
		},
	}
}

func newMatrixStopCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop all sandboxes in a matrix",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := (*ctrl).StopMatrix(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Matrix %q stopped.\n", args[0])
			return nil
		},
	}
}

func newMatrixStartCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start all sandboxes in a stopped matrix",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := (*ctrl).StartMatrix(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Matrix %q started.\n", args[0])
			return nil
		},
	}
}

func newMatrixDestroyCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:     "destroy <name>",
		Aliases: []string{"rm"},
		Short:   "Destroy a matrix and all its sandboxes",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := (*ctrl).DestroyMatrix(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Matrix %q destroyed.\n", args[0])
			return nil
		},
	}
}

// parseMembers converts "name:blueprint" strings to MatrixMember values.
func parseMembers(raw []string) ([]v1alpha1.MatrixMember, error) {
	members := make([]v1alpha1.MatrixMember, 0, len(raw))
	for _, s := range raw {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid member format %q: expected name:blueprint", s)
		}
		members = append(members, v1alpha1.MatrixMember{
			Name:      parts[0],
			Blueprint: parts[1],
		})
	}
	return members, nil
}

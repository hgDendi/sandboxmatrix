package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime/docker"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	"github.com/spf13/cobra"
)

// newLazySessionCmd creates the session command group that initializes
// the Docker runtime and controller only when a subcommand is invoked.
func newLazySessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage sessions",
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
		newSessionStartCmdLazy(&ctrl),
		newSessionEndCmdLazy(&ctrl),
		newSessionListCmdLazy(&ctrl),
		newSessionExecCmdLazy(&ctrl),
	)

	return cmd
}

func newSessionStartCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "start <sandbox-name>",
		Short: "Start a new session for a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			session, err := (*ctrl).StartSession(context.Background(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Session %q started for sandbox %q.\n", session.Metadata.Name, session.Sandbox)
			return nil
		},
	}
}

func newSessionEndCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "end <session-id>",
		Short: "End a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := (*ctrl).EndSession(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Session %q ended.\n", args[0])
			return nil
		},
	}
}

func newSessionListCmdLazy(ctrl **controller.Controller) *cobra.Command {
	var sandbox string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List sessions",
		RunE: func(_ *cobra.Command, _ []string) error {
			sessions, err := (*ctrl).ListSessions(sandbox)
			if err != nil {
				return err
			}
			if len(sessions) == 0 {
				fmt.Println("No sessions found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSANDBOX\tSTATE\tEXECS\tAGE")
			for _, s := range sessions {
				age := time.Since(s.Metadata.CreatedAt).Truncate(time.Second)
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", s.Metadata.Name, s.Sandbox, s.State, s.ExecCount, age)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVarP(&sandbox, "sandbox", "s", "", "Filter by sandbox name")
	return cmd
}

func newSessionExecCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "exec <session-id> -- <command...>",
		Short: "Execute a command in a session",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			sessionID := args[0]
			cmdArgs := args[1:]
			if len(cmdArgs) == 0 {
				cmdArgs = []string{"/bin/sh"}
			}
			result, err := (*ctrl).ExecInSession(context.Background(), sessionID, &runtime.ExecConfig{
				Cmd:    cmdArgs,
				Stdout: os.Stdout,
				Stderr: os.Stderr,
			})
			if err != nil {
				return err
			}
			if result.ExitCode != 0 {
				return fmt.Errorf("exit code: %d", result.ExitCode)
			}
			return nil
		},
	}
}

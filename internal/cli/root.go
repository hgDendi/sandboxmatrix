// Package cli implements the smx command-line interface.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime/docker"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root smx command.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "smx",
		Short: "sandboxMatrix - AI sandbox orchestrator",
		Long: `sandboxMatrix (smx) is an open-source, local-first AI sandbox orchestrator.

It provides Kubernetes-inspired abstractions for creating and managing
isolated development environments with pluggable runtime backends.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Non-runtime commands always available.
	cmd.AddCommand(
		newVersionCmd(),
		newBlueprintCmd(),
	)

	// Runtime commands require Docker; initialize lazily on use.
	sandboxCmd := newLazySandboxCmd()
	cmd.AddCommand(sandboxCmd)

	return cmd
}

// newLazySandboxCmd creates the sandbox command group that initializes
// the Docker runtime and controller only when a subcommand is invoked.
func newLazySandboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sandbox",
		Aliases: []string{"sb"},
		Short:   "Manage sandboxes",
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
		ctrl = controller.New(rt, store)
		// Reconcile state from Docker containers.
		if err := ctrl.Reconcile(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: state reconciliation failed: %v\n", err)
		}
		return nil
	}

	// We use PersistentPreRunE so it runs before any subcommand.
	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		return initController()
	}

	// Register subcommands with a getter that returns the initialized controller.
	// We use a wrapper to add commands that reference ctrl after init.
	cmd.AddCommand(
		newSandboxCreateCmdLazy(&ctrl),
		newSandboxListCmdLazy(&ctrl),
		newSandboxStopCmdLazy(&ctrl),
		newSandboxStartCmdLazy(&ctrl),
		newSandboxDestroyCmdLazy(&ctrl),
		newSandboxExecCmdLazy(&ctrl),
		newSandboxInspectCmdLazy(&ctrl),
	)

	return cmd
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

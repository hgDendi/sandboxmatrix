// Package cli implements the smx command-line interface.
package cli

import (
	"os"

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

	cmd.AddCommand(
		newVersionCmd(),
		newBlueprintCmd(),
	)

	return cmd
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

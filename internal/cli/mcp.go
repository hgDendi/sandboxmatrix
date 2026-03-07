package cli

import (
	"context"
	"fmt"
	"os"

	mcpserver "github.com/hg-dendi/sandboxmatrix/internal/agent/mcp"
	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime/docker"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	"github.com/spf13/cobra"
)

// newMCPCmd creates the "mcp" command group with its subcommands.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP (Model Context Protocol) server for AI agents",
	}

	cmd.AddCommand(newMCPServeCmd())
	return cmd
}

// newMCPServeCmd creates the "mcp serve" command that starts the MCP server
// on stdio, allowing AI agents to manage sandboxes via the Model Context Protocol.
func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server on stdio",
		Long: `Start an MCP (Model Context Protocol) server that communicates over stdin/stdout.

This allows AI agents to create, list, exec into, start, stop, and destroy
sandboxes using the standard MCP tool-calling interface.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Initialize Docker runtime.
			rt, err := docker.New()
			if err != nil {
				return fmt.Errorf("initialize Docker runtime: %w\n\nIs Docker running?", err)
			}

			// Initialize persistent state store.
			store, err := state.NewFileStore()
			if err != nil {
				return fmt.Errorf("initialize state store: %w", err)
			}

			// Initialize session store.
			sessions, err := state.NewFileSessionStore()
			if err != nil {
				return fmt.Errorf("initialize session store: %w", err)
			}

			// Create the controller.
			ctrl := controller.New(rt, store, sessions)

			// Reconcile state from Docker containers.
			if err := ctrl.Reconcile(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "warning: state reconciliation failed: %v\n", err)
			}

			// Create and start the MCP server on stdio.
			srv := mcpserver.NewServer(ctrl)
			return srv.ServeStdio()
		},
	}
}

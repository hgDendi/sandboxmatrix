package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime/docker"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	"github.com/hg-dendi/sandboxmatrix/internal/web"
	"github.com/spf13/cobra"
)

func newDashboardCmd() *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Start the web dashboard",
		Long:  `Start an embedded web dashboard for managing sandboxes, matrices, and sessions.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Initialize runtime and stores.
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
			ctrl := controller.New(rt, store, sessions, matrices)

			// Reconcile state from Docker containers.
			if err := ctrl.Reconcile(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "warning: state reconciliation failed: %v\n", err)
			}

			// Start the dashboard.
			dash := web.NewDashboard(ctrl, addr)
			if err := dash.Start(); err != nil {
				return fmt.Errorf("start dashboard: %w", err)
			}

			fmt.Printf("sandboxMatrix dashboard running at http://localhost%s\n", addr)

			// Wait for interrupt signal.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			<-sigCh

			fmt.Println("\nShutting down dashboard...")
			ctx, cancel := context.WithTimeout(context.Background(), 5e9) // 5 seconds
			defer cancel()
			return dash.Shutdown(ctx)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":9090", "Address to listen on")
	return cmd
}

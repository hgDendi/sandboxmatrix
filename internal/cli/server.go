package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime/docker"
	"github.com/hg-dendi/sandboxmatrix/internal/server"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	"github.com/spf13/cobra"
)

func newServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage the sandboxMatrix API server",
	}

	cmd.AddCommand(newServerStartCmd())
	return cmd
}

func newServerStartCmd() *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the REST API server",
		Long: `Start an HTTP REST API server that exposes sandboxMatrix operations
over HTTP. SDKs and external clients can interact with sandboxMatrix
through this server instead of subprocess CLI calls.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Initialize Docker runtime.
			rt, err := docker.New()
			if err != nil {
				return fmt.Errorf("initialize Docker runtime: %w\n\nIs Docker running?", err)
			}

			// Initialize state stores.
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

			// Create the controller.
			ctrl := controller.New(rt, store, sessions, matrices)

			// Propagate version info to the server package.
			server.ServerVersion = Version
			server.ServerCommit = Commit
			server.ServerBuildDate = BuildDate

			// Create and start the HTTP server.
			srv := server.New(ctrl, addr)

			// Graceful shutdown on SIGINT/SIGTERM.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			errCh := make(chan error, 1)
			go func() {
				errCh <- srv.Start()
			}()

			select {
			case err := <-errCh:
				return err
			case sig := <-sigCh:
				fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := srv.Shutdown(ctx); err != nil {
					return fmt.Errorf("shutdown: %w", err)
				}
				fmt.Println("Server stopped gracefully.")
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "Address to listen on (e.g. :8080, 127.0.0.1:9090)")
	return cmd
}

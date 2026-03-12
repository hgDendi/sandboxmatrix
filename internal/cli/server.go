package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/auth"
	"github.com/hg-dendi/sandboxmatrix/internal/config"
	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/observability"
	"github.com/hg-dendi/sandboxmatrix/internal/quota"
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
			// Load configuration.
			cfg, err := config.Load()
			if err != nil {
				slog.Warn("failed to load config, using defaults", "error", err)
				cfg = config.DefaultConfig()
			}

			// Initialize tracing (noop if disabled).
			tracingShutdown, err := observability.InitTracing(context.Background(), cfg.Tracing)
			if err != nil {
				return fmt.Errorf("initialize tracing: %w", err)
			}
			if cfg.Tracing.Enabled {
				slog.Info("OpenTelemetry tracing enabled",
					"endpoint", cfg.Tracing.Endpoint,
					"protocol", cfg.Tracing.Protocol,
				)
			}

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

			// Initialize team store and quota manager.
			teams, err := state.NewFileTeamStore()
			if err != nil {
				return fmt.Errorf("initialize team store: %w", err)
			}
			qm := quota.New(teams, store, matrices, sessions)

			// Create the controller with quota enforcement.
			ctrl := controller.New(rt, store, sessions, matrices,
				controller.WithQuotaChecker(qm))

			// Propagate version info to the server package.
			server.ServerVersion = Version
			server.ServerCommit = Commit
			server.ServerBuildDate = BuildDate

			// Build server options.
			serverOpts := []server.Option{
				server.WithTeams(teams, qm),
			}

			// Initialize OIDC if configured.
			if cfg.OIDC.Enabled {
				oidcCfg := auth.OIDCConfig{
					Provider:     cfg.OIDC.Provider,
					Issuer:       cfg.OIDC.Issuer,
					ClientID:     cfg.OIDC.ClientID,
					ClientSecret: cfg.OIDC.ClientSecret,
					RedirectURL:  cfg.OIDC.RedirectURL,
					Scopes:       cfg.OIDC.Scopes,
				}
				oidcProvider, oidcErr := auth.NewOIDCProvider(oidcCfg)
				if oidcErr != nil {
					slog.Error("failed to initialize OIDC provider", "error", oidcErr)
				} else {
					// Parse JWT TTLs.
					var accessTTL, refreshTTL time.Duration
					if cfg.JWT.AccessTokenTTL != "" {
						if d, parseErr := time.ParseDuration(cfg.JWT.AccessTokenTTL); parseErr == nil {
							accessTTL = d
						}
					}
					if cfg.JWT.RefreshTokenTTL != "" {
						if d, parseErr := time.ParseDuration(cfg.JWT.RefreshTokenTTL); parseErr == nil {
							refreshTTL = d
						}
					}

					jwtSvc := auth.NewJWTService(auth.JWTConfig{
						SigningKey:      cfg.JWT.SigningKey,
						Issuer:          cfg.JWT.Issuer,
						AccessTokenTTL:  accessTTL,
						RefreshTokenTTL: refreshTTL,
					})

					serverOpts = append(serverOpts, server.WithOIDC(oidcProvider, jwtSvc, cfg.OIDC.RoleMapping))
					slog.Info("OIDC authentication enabled", "provider", cfg.OIDC.Provider)
				}
			}

			// Create and start the HTTP server.
			srv := server.New(ctrl, addr, serverOpts...)

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
				slog.Info("received signal, shutting down", "signal", sig)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := srv.Shutdown(ctx); err != nil {
					return fmt.Errorf("shutdown: %w", err)
				}
				// Flush any pending trace spans.
				if err := tracingShutdown(ctx); err != nil {
					slog.Warn("tracing shutdown error", "error", err)
				}
				slog.Info("server stopped gracefully")
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "Address to listen on (e.g. :8080, 127.0.0.1:9090)")
	return cmd
}

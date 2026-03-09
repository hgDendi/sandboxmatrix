package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/hg-dendi/sandboxmatrix/internal/pool"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime/docker"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	"github.com/spf13/cobra"
)

// newPoolCmd creates the pool command group for managing pre-warmed sandbox pools.
func newPoolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pool",
		Short: "Manage pre-warmed sandbox pools",
	}

	cmd.AddCommand(
		newPoolWarmCmd(),
		newStatsCmd(),
		newPoolDrainCmd(),
	)

	return cmd
}

// newPoolWarmCmd creates the "pool warm" command which creates MinReady
// containers immediately and exits.
func newPoolWarmCmd() *cobra.Command {
	var (
		blueprintPath string
		minReady      int
		maxSize       int
	)

	cmd := &cobra.Command{
		Use:   "warm",
		Short: "Create pre-warmed sandbox instances for a blueprint",
		Long: `Create MinReady pre-warmed sandbox containers for the given blueprint.
The containers are created and started immediately, then the command exits.
Warm containers are labeled for later identification by pool stats and drain.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if blueprintPath == "" {
				return fmt.Errorf("--blueprint is required")
			}
			if minReady <= 0 {
				return fmt.Errorf("--min must be > 0")
			}

			rt, err := docker.New()
			if err != nil {
				return fmt.Errorf("initialize Docker runtime: %w\n\nIs Docker running?", err)
			}
			store := state.NewMemoryStore()

			p := pool.New(rt, store)
			if err := p.Configure(pool.Config{
				BlueprintPath: blueprintPath,
				MinReady:      minReady,
				MaxSize:       maxSize,
			}); err != nil {
				return fmt.Errorf("configure pool: %w", err)
			}

			fmt.Printf("Warming %d instances for blueprint %s...\n", minReady, blueprintPath)

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			if err := p.Warm(ctx); err != nil {
				return fmt.Errorf("warm pool: %w", err)
			}

			// Wait briefly for the warming goroutines to complete, then
			// verify by checking stats.
			// Since Warm starts goroutines, we just drain (which waits for
			// goroutines to stop) after filling completes.
			// For a non-daemon approach, we manually claim and re-check.
			// A simpler approach: create MinReady containers directly.

			// Create containers synchronously.
			for i := 0; i < minReady; i++ {
				select {
				case <-ctx.Done():
					slog.Info("pool warm interrupted", "completed", i, "total", minReady)
					return nil
				default:
				}
				id, claimErr := p.Claim(ctx, blueprintPath)
				if claimErr != nil {
					return fmt.Errorf("create warm instance %d: %w", i+1, claimErr)
				}
				if releaseErr := p.Release(ctx, id, blueprintPath); releaseErr != nil {
					return fmt.Errorf("release warm instance %d: %w", i+1, releaseErr)
				}
			}

			// Stop background goroutines (ignore error — drain is best-effort here).

			fmt.Printf("Successfully warmed %d instances.\n", minReady)
			return nil
		},
	}

	cmd.Flags().StringVarP(&blueprintPath, "blueprint", "b", "", "Path to blueprint YAML file")
	cmd.Flags().IntVar(&minReady, "min", 2, "Minimum number of warm instances to create")
	cmd.Flags().IntVar(&maxSize, "max", 5, "Maximum pool size")
	return cmd
}

// newStatsCmd creates the "pool stats" command which lists containers
// with pool labels and shows counts.
func newStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show pool statistics from running containers",
		Long: `Lists all containers labeled as pool instances and shows counts
grouped by blueprint.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			rt, err := docker.New()
			if err != nil {
				return fmt.Errorf("initialize Docker runtime: %w\n\nIs Docker running?", err)
			}

			return showStats(rt)
		},
	}
}

// showStats queries the runtime for pool-labeled containers and prints a
// summary table.
func showStats(rt runtime.Runtime) error {
	ctx := context.Background()
	containers, err := rt.List(ctx)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	type blueprintStats struct {
		blueprint string
		running   int
		stopped   int
		total     int
	}

	stats := make(map[string]*blueprintStats)

	for _, c := range containers {
		if c.Labels[pool.LabelPool] != "true" {
			continue
		}
		bpPath := c.Labels[pool.LabelPoolBlueprint]
		if bpPath == "" {
			bpPath = "(unknown)"
		}

		s, ok := stats[bpPath]
		if !ok {
			s = &blueprintStats{blueprint: bpPath}
			stats[bpPath] = s
		}
		s.total++
		if c.State == "running" {
			s.running++
		} else {
			s.stopped++
		}
	}

	if len(stats) == 0 {
		fmt.Println("No pool instances found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BLUEPRINT\tRUNNING\tSTOPPED\tTOTAL")
	for _, s := range stats {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\n", s.blueprint, s.running, s.stopped, s.total)
	}
	return w.Flush()
}

// newPoolDrainCmd creates the "pool drain" command which destroys all
// containers with pool labels.
func newPoolDrainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drain",
		Short: "Destroy all pre-warmed pool instances",
		Long:  `Finds all containers labeled as pool instances and destroys them.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			rt, err := docker.New()
			if err != nil {
				return fmt.Errorf("initialize Docker runtime: %w\n\nIs Docker running?", err)
			}

			return drainPoolContainers(rt)
		},
	}
}

// drainPoolContainers finds and destroys all containers with pool labels.
func drainPoolContainers(rt runtime.Runtime) error {
	ctx := context.Background()
	containers, err := rt.List(ctx)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	var count int
	for _, c := range containers {
		if c.Labels[pool.LabelPool] != "true" {
			continue
		}
		fmt.Printf("Destroying pool instance %s (%s)...\n", c.ID, c.Labels[pool.LabelPoolBlueprint])
		if err := rt.Destroy(ctx, c.ID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to destroy %s: %v\n", c.ID, err)
			continue
		}
		count++
	}

	if count == 0 {
		fmt.Println("No pool instances found to drain.")
	} else {
		fmt.Printf("Drained %d pool instance(s).\n", count)
	}
	return nil
}

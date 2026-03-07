package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/spf13/cobra"
)

func newSandboxCreateCmdLazy(ctrl **controller.Controller) *cobra.Command {
	var (
		blueprintPath string
		name          string
		workspace     string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create and start a new sandbox",
		RunE: func(_ *cobra.Command, _ []string) error {
			if blueprintPath == "" {
				return fmt.Errorf("--blueprint is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required")
			}

			fmt.Printf("Creating sandbox %q from blueprint %s...\n", name, blueprintPath)
			sb, err := (*ctrl).Create(context.Background(), controller.CreateOptions{
				Name:          name,
				BlueprintPath: blueprintPath,
				WorkspaceDir:  workspace,
			})
			if err != nil {
				return err
			}
			runtimeID := sb.Status.RuntimeID
			if len(runtimeID) > 12 {
				runtimeID = runtimeID[:12]
			}
			fmt.Printf("Sandbox %q is %s (runtime: %s)\n", sb.Metadata.Name, sb.Status.State, runtimeID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&blueprintPath, "blueprint", "b", "", "Path to blueprint YAML file")
	cmd.Flags().StringVarP(&name, "name", "n", "", "Sandbox name")
	cmd.Flags().StringVarP(&workspace, "workspace", "w", "", "Local directory to mount as workspace")
	return cmd
}

func newSandboxListCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all sandboxes",
		RunE: func(_ *cobra.Command, _ []string) error {
			sandboxes, err := (*ctrl).List()
			if err != nil {
				return err
			}
			if len(sandboxes) == 0 {
				fmt.Println("No sandboxes found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATE\tBLUEPRINT\tRUNTIME ID\tAGE")
			for _, sb := range sandboxes {
				age := time.Since(sb.Metadata.CreatedAt).Truncate(time.Second)
				runtimeID := sb.Status.RuntimeID
				if len(runtimeID) > 12 {
					runtimeID = runtimeID[:12]
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					sb.Metadata.Name,
					sb.Status.State,
					sb.Spec.BlueprintRef,
					runtimeID,
					age,
				)
			}
			return w.Flush()
		},
	}
}

func newSandboxStopCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a running sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := (*ctrl).Stop(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Sandbox %q stopped.\n", args[0])
			return nil
		},
	}
}

func newSandboxStartCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Start a stopped sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := (*ctrl).Start(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Sandbox %q started.\n", args[0])
			return nil
		},
	}
}

func newSandboxDestroyCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:     "destroy <name>",
		Aliases: []string{"rm"},
		Short:   "Destroy a sandbox",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := (*ctrl).Destroy(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Sandbox %q destroyed.\n", args[0])
			return nil
		},
	}
}

func newSandboxExecCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "exec <name> -- <command...>",
		Short: "Execute a command in a running sandbox",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			cmdArgs := args[1:]

			if len(cmdArgs) == 0 {
				cmdArgs = []string{"/bin/sh"}
			}

			result, err := (*ctrl).Exec(context.Background(), name, runtime.ExecConfig{
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

func newSandboxInspectCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <name>",
		Short: "Show detailed sandbox information",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			sb, err := (*ctrl).Get(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Name:       %s\n", sb.Metadata.Name)
			fmt.Printf("State:      %s\n", sb.Status.State)
			fmt.Printf("Blueprint:  %s\n", sb.Spec.BlueprintRef)
			fmt.Printf("Runtime ID: %s\n", sb.Status.RuntimeID)
			if sb.Status.IP != "" {
				fmt.Printf("IP:         %s\n", sb.Status.IP)
			}
			fmt.Printf("Created:    %s\n", sb.Metadata.CreatedAt.Format(time.RFC3339))
			if sb.Status.StartedAt != nil {
				fmt.Printf("Started:    %s\n", sb.Status.StartedAt.Format(time.RFC3339))
			}
			if sb.Status.StoppedAt != nil {
				fmt.Printf("Stopped:    %s\n", sb.Status.StoppedAt.Format(time.RFC3339))
			}
			if sb.Spec.Workspace.Source != "" {
				fmt.Printf("Workspace:  %s -> %s\n", sb.Spec.Workspace.Source, sb.Spec.Workspace.MountPath)
			}
			if sb.Status.Message != "" {
				fmt.Printf("Message:    %s\n", sb.Status.Message)
			}
			return nil
		},
	}
}

func newSandboxStatsCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "stats <name>",
		Short: "Show resource usage statistics for a running sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			stats, err := (*ctrl).Stats(context.Background(), args[0])
			if err != nil {
				return err
			}
			memUsageMiB := float64(stats.MemoryUsage) / (1024 * 1024)
			memLimitMiB := float64(stats.MemoryLimit) / (1024 * 1024)
			var memPercent float64
			if stats.MemoryLimit > 0 {
				memPercent = float64(stats.MemoryUsage) / float64(stats.MemoryLimit) * 100.0
			}
			fmt.Printf("CPU:     %.1f%%\n", stats.CPUUsage)
			fmt.Printf("Memory:  %.1f MiB / %.1f MiB (%.1f%%)\n", memUsageMiB, memLimitMiB, memPercent)
			return nil
		},
	}
}

func newSandboxSnapshotCmdLazy(ctrl **controller.Controller) *cobra.Command {
	var tag string

	cmd := &cobra.Command{
		Use:   "snapshot <name>",
		Short: "Create a point-in-time snapshot of a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			if tag == "" {
				tag = time.Now().Format("20060102-150405")
			}
			fmt.Printf("Creating snapshot of sandbox %q with tag %q...\n", name, tag)
			snapshotID, err := (*ctrl).Snapshot(context.Background(), name, tag)
			if err != nil {
				return err
			}
			id := snapshotID
			if len(id) > 12 {
				id = id[:12]
			}
			fmt.Printf("Snapshot created: %s\n", id)
			return nil
		},
	}

	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Snapshot tag (default: timestamp)")
	return cmd
}

func newSandboxRestoreCmdLazy(ctrl **controller.Controller) *cobra.Command {
	var (
		snapshotID string
		newName    string
	)

	cmd := &cobra.Command{
		Use:   "restore <name>",
		Short: "Create a new sandbox from a snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			if snapshotID == "" {
				return fmt.Errorf("--snapshot is required")
			}
			if newName == "" {
				return fmt.Errorf("--name is required")
			}
			fmt.Printf("Restoring sandbox %q from snapshot %s as %q...\n", name, snapshotID, newName)
			sb, err := (*ctrl).Restore(context.Background(), name, snapshotID, newName)
			if err != nil {
				return err
			}
			runtimeID := sb.Status.RuntimeID
			if len(runtimeID) > 12 {
				runtimeID = runtimeID[:12]
			}
			fmt.Printf("Sandbox %q is %s (runtime: %s)\n", sb.Metadata.Name, sb.Status.State, runtimeID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&snapshotID, "snapshot", "s", "", "Snapshot ID to restore from")
	cmd.Flags().StringVarP(&newName, "name", "n", "", "Name for the new sandbox")
	return cmd
}

func newSandboxSnapshotsCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "snapshots <name>",
		Short: "List snapshots of a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			snapshots, err := (*ctrl).ListSnapshots(context.Background(), name)
			if err != nil {
				return err
			}
			if len(snapshots) == 0 {
				fmt.Println("No snapshots found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tTAG\tCREATED\tSIZE")
			for _, snap := range snapshots {
				id := snap.ID
				if len(id) > 12 {
					id = id[:12]
				}
				size := fmt.Sprintf("%.1f MB", float64(snap.Size)/(1024*1024))
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					id,
					snap.Tag,
					snap.CreatedAt.Format(time.RFC3339),
					size,
				)
			}
			return w.Flush()
		},
	}
}

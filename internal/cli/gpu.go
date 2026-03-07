package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/spf13/cobra"
)

func newSandboxGPUCheckCmdLazy(ctrl **controller.Controller) *cobra.Command {
	return &cobra.Command{
		Use:   "gpu-check <name>",
		Short: "Check GPU availability inside a sandbox (runs nvidia-smi)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]

			fmt.Printf("Checking GPU status in sandbox %q...\n", name)
			result, err := (*ctrl).Exec(context.Background(), name, &runtime.ExecConfig{
				Cmd:    []string{"nvidia-smi"},
				Stdout: os.Stdout,
				Stderr: os.Stderr,
			})
			if err != nil {
				return fmt.Errorf("gpu check failed: %w", err)
			}
			if result.ExitCode != 0 {
				return fmt.Errorf("nvidia-smi exited with code %d (GPU may not be available)", result.ExitCode)
			}
			return nil
		},
	}
}

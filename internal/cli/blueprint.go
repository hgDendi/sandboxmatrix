package cli

import (
	"fmt"

	"github.com/hg-dendi/sandboxmatrix/pkg/blueprint"
	"github.com/spf13/cobra"
)

func newBlueprintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "blueprint",
		Aliases: []string{"bp"},
		Short:   "Manage blueprints",
	}

	cmd.AddCommand(
		newBlueprintValidateCmd(),
		newBlueprintInspectCmd(),
	)

	return cmd
}

func newBlueprintValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a blueprint YAML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			bp, errs := blueprint.ValidateFile(args[0])
			if len(errs) > 0 {
				fmt.Printf("Blueprint %q has %d validation error(s):\n", args[0], len(errs))
				for _, err := range errs {
					fmt.Printf("  - %s\n", err)
				}
				return fmt.Errorf("validation failed")
			}
			fmt.Printf("Blueprint %q (%s) is valid.\n", bp.Metadata.Name, args[0])
			return nil
		},
	}
}

func newBlueprintInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <file>",
		Short: "Display details of a blueprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			bp, err := blueprint.ParseFile(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Name:      %s\n", bp.Metadata.Name)
			fmt.Printf("Version:   %s\n", bp.Metadata.Version)
			fmt.Printf("Base:      %s\n", bp.Spec.Base)
			fmt.Printf("Runtime:   %s\n", bp.Spec.Runtime)
			if bp.Spec.Resources.CPU != "" {
				fmt.Printf("CPU:       %s\n", bp.Spec.Resources.CPU)
			}
			if bp.Spec.Resources.Memory != "" {
				fmt.Printf("Memory:    %s\n", bp.Spec.Resources.Memory)
			}
			if bp.Spec.Resources.Disk != "" {
				fmt.Printf("Disk:      %s\n", bp.Spec.Resources.Disk)
			}
			if len(bp.Spec.Setup) > 0 {
				fmt.Printf("Setup:     %d step(s)\n", len(bp.Spec.Setup))
			}
			if len(bp.Spec.Toolchains) > 0 {
				fmt.Printf("Toolchains: %d\n", len(bp.Spec.Toolchains))
			}
			if bp.Spec.Workspace.MountPath != "" {
				fmt.Printf("Workspace: %s\n", bp.Spec.Workspace.MountPath)
			}
			if len(bp.Spec.Network.Expose) > 0 {
				fmt.Printf("Ports:     %v\n", bp.Spec.Network.Expose)
			}
			return nil
		},
	}
}

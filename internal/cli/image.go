package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/hg-dendi/sandboxmatrix/internal/image"
	"github.com/hg-dendi/sandboxmatrix/internal/runtime/docker"
	"github.com/spf13/cobra"
)

// newLazyImageCmd creates the image command group that initializes
// the Docker runtime only when a subcommand is invoked.
func newLazyImageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Manage pre-built blueprint images",
	}

	var builder *image.Builder

	initBuilder := func() error {
		if builder != nil {
			return nil
		}
		rt, err := docker.New()
		if err != nil {
			return fmt.Errorf("initialize Docker runtime: %w\n\nIs Docker running?", err)
		}
		builder = image.NewBuilder(rt)
		return nil
	}

	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		return initBuilder()
	}

	cmd.AddCommand(
		newImageBuildCmdLazy(&builder),
		newImageListCmdLazy(&builder),
		newImageCleanCmdLazy(&builder),
	)

	return cmd
}

func newImageBuildCmdLazy(builder **image.Builder) *cobra.Command {
	var blueprintPath string

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a Docker image from a blueprint for faster sandbox creation",
		RunE: func(_ *cobra.Command, _ []string) error {
			if blueprintPath == "" {
				return fmt.Errorf("--blueprint is required")
			}

			fmt.Printf("Building image from blueprint %s...\n", blueprintPath)
			result, err := (*builder).Build(context.Background(), blueprintPath)
			if err != nil {
				return err
			}
			imageID := result.ImageID
			if len(imageID) > 12 {
				imageID = imageID[:12]
			}
			fmt.Printf("Image built successfully.\n")
			fmt.Printf("  Blueprint: %s\n", result.Blueprint)
			fmt.Printf("  Tag:       %s\n", result.ImageTag)
			fmt.Printf("  Image ID:  %s\n", imageID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&blueprintPath, "blueprint", "b", "", "Path to blueprint YAML file")
	return cmd
}

func newImageListCmdLazy(builder **image.Builder) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all pre-built blueprint images",
		RunE: func(_ *cobra.Command, _ []string) error {
			images, err := (*builder).ListBuiltImages(context.Background())
			if err != nil {
				return err
			}
			if len(images) == 0 {
				fmt.Println("No built images found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TAG\tBLUEPRINT\tIMAGE ID\tSIZE")
			for _, img := range images {
				id := img.ID
				if len(id) > 12 {
					id = id[:12]
				}
				size := fmt.Sprintf("%.1f MB", float64(img.Size)/(1024*1024))
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					img.Tag,
					img.Blueprint,
					id,
					size,
				)
			}
			return w.Flush()
		},
	}
}

func newImageCleanCmdLazy(builder **image.Builder) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove all pre-built blueprint images",
		RunE: func(_ *cobra.Command, _ []string) error {
			removed, err := (*builder).Clean(context.Background())
			if err != nil {
				return err
			}
			fmt.Printf("Removed %d built image(s).\n", removed)
			return nil
		},
	}
}

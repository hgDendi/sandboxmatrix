package cli

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build-time variables set via ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

func newVersionCmd() *cobra.Command {
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			info := versionInfo{
				Version:   Version,
				Commit:    Commit,
				BuildDate: BuildDate,
				GoVersion: runtime.Version(),
				OS:        runtime.GOOS,
				Arch:      runtime.GOARCH,
			}

			if outputJSON {
				data, err := json.MarshalIndent(info, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(data))
				return nil
			}

			fmt.Printf("smx version %s\n", info.Version)
			fmt.Printf("  commit:    %s\n", info.Commit)
			fmt.Printf("  built:     %s\n", info.BuildDate)
			fmt.Printf("  go:        %s\n", info.GoVersion)
			fmt.Printf("  os/arch:   %s/%s\n", info.OS, info.Arch)
			return nil
		},
	}

	cmd.Flags().BoolVar(&outputJSON, "json", false, "Output in JSON format")
	return cmd
}

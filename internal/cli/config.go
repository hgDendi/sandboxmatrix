package cli

import (
	"fmt"
	"strings"

	"github.com/hg-dendi/sandboxmatrix/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage sandboxMatrix configuration",
	}
	cmd.AddCommand(newConfigShowCmd(), newConfigSetCmd(), newConfigPathCmd(), newConfigInitCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display current configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		},
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show config file path",
		RunE: func(_ *cobra.Command, _ []string) error {
			path, err := config.FilePath()
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		},
	}
}

func newConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default config file",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := config.Save(config.DefaultConfig()); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			path, _ := config.FilePath()
			fmt.Printf("Config file created at %s\n", path)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long:  "Set a configuration value. Keys: logLevel, defaultRuntime, server.addr, dashboard.addr, pool.minReady, pool.maxSize, blueprintDir, stateDir",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			switch strings.ToLower(key) {
			case "loglevel":
				cfg.LogLevel = value
			case "defaultruntime":
				cfg.DefaultRuntime = value
			case "server.addr":
				cfg.Server.Addr = value
			case "dashboard.addr":
				cfg.Dashboard.Addr = value
			case "blueprintdir":
				cfg.BlueprintDir = value
			case "statedir":
				cfg.StateDir = value
			default:
				return fmt.Errorf("unknown config key: %s", key)
			}

			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("Set %s = %s\n", key, value)
			return nil
		},
	}
}

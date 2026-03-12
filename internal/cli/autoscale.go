package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

func newAutoscaleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autoscale",
		Short: "Manage the dynamic resource autoscaler",
	}

	cmd.AddCommand(
		newAutoscaleStatusCmd(),
		newAutoscaleEnableCmd(),
		newAutoscaleDisableCmd(),
	)
	return cmd
}

func newAutoscaleStatusCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show autoscaler status and sandbox states",
		RunE: func(_ *cobra.Command, _ []string) error {
			url := fmt.Sprintf("http://%s/api/v1/autoscale/status", addr)
			resp, err := http.Get(url)
			if err != nil {
				return fmt.Errorf("connect to server: %w\n\nIs the server running? (smx server start)", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
			}

			var status map[string]interface{}
			if err := json.Unmarshal(body, &status); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			prettyJSON, _ := json.MarshalIndent(status, "", "  ")
			fmt.Println(string(prettyJSON))
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "localhost:8080", "Server address")
	return cmd
}

func newAutoscaleEnableCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable the autoscaler",
		RunE: func(_ *cobra.Command, _ []string) error {
			url := fmt.Sprintf("http://%s/api/v1/autoscale/enable", addr)
			resp, err := http.Post(url, "application/json", http.NoBody)
			if err != nil {
				return fmt.Errorf("connect to server: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
			}

			fmt.Println("Autoscaler enabled")
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "localhost:8080", "Server address")
	return cmd
}

func newAutoscaleDisableCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable the autoscaler and restore all sandboxes",
		RunE: func(_ *cobra.Command, _ []string) error {
			url := fmt.Sprintf("http://%s/api/v1/autoscale/disable", addr)
			resp, err := http.Post(url, "application/json", http.NoBody)
			if err != nil {
				return fmt.Errorf("connect to server: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("read response: %w", err)
			}

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(body))
			}

			fmt.Println("Autoscaler disabled, all sandboxes restored to original limits")
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "localhost:8080", "Server address")
	return cmd
}

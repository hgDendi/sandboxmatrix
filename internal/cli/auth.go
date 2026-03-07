package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/hg-dendi/sandboxmatrix/internal/auth"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"github.com/spf13/cobra"
)

// defaultAuthDir returns the path to ~/.sandboxmatrix where auth state
// is persisted.
func defaultAuthDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".sandboxmatrix"), nil
}

// usersFilePath returns the default path to the users file.
func usersFilePath() (string, error) {
	dir, err := defaultAuthDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "users.json"), nil
}

// auditFilePath returns the default path to the audit log file.
func auditFilePath() (string, error) {
	dir, err := defaultAuthDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "audit.log"), nil
}

// loadRBAC creates an RBAC instance and loads persisted users from disk.
func loadRBAC() (*auth.RBAC, error) {
	rbac := auth.New()

	path, err := usersFilePath()
	if err != nil {
		return rbac, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return rbac, nil // No users file yet.
		}
		return rbac, fmt.Errorf("reading users file: %w", err)
	}

	var users []*v1alpha1.User
	if err := json.Unmarshal(data, &users); err != nil {
		return rbac, fmt.Errorf("parsing users file: %w", err)
	}

	for _, u := range users {
		if err := rbac.AddUser(u); err != nil {
			return rbac, fmt.Errorf("loading user %q: %w", u.Name, err)
		}
	}

	return rbac, nil
}

// saveUsers persists the current RBAC users to disk.
func saveUsers(rbac *auth.RBAC) error {
	path, err := usersFilePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating auth directory: %w", err)
	}

	// Get all users with full tokens for persistence.
	// ListUsers redacts tokens, so we need to re-fetch individually.
	listed := rbac.ListUsers()
	var users []*v1alpha1.User
	for _, u := range listed {
		full, err := rbac.GetUser(u.Name)
		if err != nil {
			continue
		}
		users = append(users, full)
	}

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling users: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing users file: %w", err)
	}

	return nil
}

// newAuthCmd creates the auth command group.
func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication, users, and audit logs",
	}

	cmd.AddCommand(
		newAuthAddUserCmd(),
		newAuthListUsersCmd(),
		newAuthRemoveUserCmd(),
		newAuthAuditCmd(),
	)

	return cmd
}

// newAuthAddUserCmd creates the "auth add-user" command.
func newAuthAddUserCmd() *cobra.Command {
	var role string

	cmd := &cobra.Command{
		Use:   "add-user <name>",
		Short: "Add a new user and generate a token",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]

			rbac, err := loadRBAC()
			if err != nil {
				return fmt.Errorf("load auth state: %w", err)
			}

			user := &v1alpha1.User{
				Name: name,
				Role: v1alpha1.Role(role),
			}

			if err := rbac.AddUser(user); err != nil {
				return err
			}

			// Retrieve the user to get the auto-generated token.
			created, err := rbac.GetUser(name)
			if err != nil {
				return err
			}

			if err := saveUsers(rbac); err != nil {
				return fmt.Errorf("save auth state: %w", err)
			}

			fmt.Printf("User %q created with role %q.\n", created.Name, created.Role)
			fmt.Printf("Token: %s\n", created.Token)
			fmt.Println()
			fmt.Println("Store this token securely. It will not be shown again.")
			return nil
		},
	}

	cmd.Flags().StringVar(&role, "role", "viewer", "Role for the user (admin, operator, viewer)")
	return cmd
}

// newAuthListUsersCmd creates the "auth list-users" command.
func newAuthListUsersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-users",
		Short: "List all users",
		RunE: func(_ *cobra.Command, _ []string) error {
			rbac, err := loadRBAC()
			if err != nil {
				return fmt.Errorf("load auth state: %w", err)
			}

			users := rbac.ListUsers()
			if len(users) == 0 {
				fmt.Println("No users configured.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tROLE\tTOKEN")
			for _, u := range users {
				fmt.Fprintf(w, "%s\t%s\t%s\n", u.Name, u.Role, u.Token)
			}
			return w.Flush()
		},
	}
}

// newAuthRemoveUserCmd creates the "auth remove-user" command.
func newAuthRemoveUserCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-user <name>",
		Short: "Remove a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]

			rbac, err := loadRBAC()
			if err != nil {
				return fmt.Errorf("load auth state: %w", err)
			}

			if err := rbac.RemoveUser(name); err != nil {
				return err
			}

			if err := saveUsers(rbac); err != nil {
				return fmt.Errorf("save auth state: %w", err)
			}

			fmt.Printf("User %q removed.\n", name)
			return nil
		},
	}
}

// newAuthAuditCmd creates the "auth audit" command.
func newAuthAuditCmd() *cobra.Command {
	var (
		user  string
		limit int
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Show audit log entries",
		RunE: func(_ *cobra.Command, _ []string) error {
			path, err := auditFilePath()
			if err != nil {
				return err
			}

			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No audit log entries found.")
					return nil
				}
				return fmt.Errorf("reading audit log: %w", err)
			}

			// Parse JSON lines.
			var entries []v1alpha1.AuditEntry
			for _, line := range splitLines(string(data)) {
				if line == "" {
					continue
				}
				var entry v1alpha1.AuditEntry
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					continue // Skip malformed lines.
				}
				entries = append(entries, entry)
			}

			// Filter and limit.
			var filtered []v1alpha1.AuditEntry
			for i := len(entries) - 1; i >= 0; i-- {
				e := entries[i]
				if user != "" && e.User != user {
					continue
				}
				filtered = append(filtered, e)
				if limit > 0 && len(filtered) >= limit {
					break
				}
			}

			if len(filtered) == 0 {
				fmt.Println("No matching audit log entries found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TIMESTAMP\tUSER\tACTION\tRESOURCE\tRESULT\tDETAIL")
			for _, e := range filtered {
				ts := e.Timestamp.Format("2006-01-02 15:04:05")
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", ts, e.User, e.Action, e.Resource, e.Result, e.Detail)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&user, "user", "", "Filter by user name")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of entries to show")
	return cmd
}

// splitLines splits a string into lines, handling both \n and \r\n.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

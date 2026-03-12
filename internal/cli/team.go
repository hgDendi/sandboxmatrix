package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/quota"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"github.com/spf13/cobra"
)

func newTeamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage teams and resource quotas",
	}

	var teams state.TeamStore

	initTeamStore := func() error {
		if teams != nil {
			return nil
		}
		var err error
		teams, err = state.NewFileTeamStore()
		if err != nil {
			return fmt.Errorf("initialize team store: %w", err)
		}
		return nil
	}

	cmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		return initTeamStore()
	}

	cmd.AddCommand(
		newTeamCreateCmd(&teams),
		newTeamListCmd(&teams),
		newTeamInspectCmd(&teams),
		newTeamAddMemberCmd(&teams),
		newTeamRemoveMemberCmd(&teams),
		newTeamSetQuotaCmd(&teams),
	)

	return cmd
}

func newTeamCreateCmd(teams *state.TeamStore) *cobra.Command {
	var (
		quotaSandboxes int
		quotaCPU       string
		quotaMemory    string
		quotaDisk      string
		quotaGPUs      int
		quotaMatrices  int
		quotaSessions  int
		displayName    string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new team",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]

			// Check if team already exists.
			if _, err := (*teams).Get(name); err == nil {
				return fmt.Errorf("team %q already exists", name)
			}

			team := &v1alpha1.Team{
				Name:        name,
				DisplayName: displayName,
				Members:     []v1alpha1.TeamMember{},
				Quota: v1alpha1.ResourceQuota{
					MaxSandboxes: quotaSandboxes,
					MaxCPU:       quotaCPU,
					MaxMemory:    quotaMemory,
					MaxDisk:      quotaDisk,
					MaxGPUs:      quotaGPUs,
					MaxMatrices:  quotaMatrices,
					MaxSessions:  quotaSessions,
				},
				CreatedAt: time.Now(),
			}

			if err := (*teams).Save(team); err != nil {
				return err
			}
			fmt.Printf("Team %q created.\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&displayName, "display-name", "", "Display name for the team")
	cmd.Flags().IntVar(&quotaSandboxes, "quota-sandboxes", 0, "Max sandboxes (0 = unlimited)")
	cmd.Flags().StringVar(&quotaCPU, "quota-cpu", "", "Max CPU cores (e.g. \"8\")")
	cmd.Flags().StringVar(&quotaMemory, "quota-memory", "", "Max memory (e.g. \"16G\")")
	cmd.Flags().StringVar(&quotaDisk, "quota-disk", "", "Max disk (e.g. \"100G\")")
	cmd.Flags().IntVar(&quotaGPUs, "quota-gpus", 0, "Max GPUs (0 = unlimited)")
	cmd.Flags().IntVar(&quotaMatrices, "quota-matrices", 0, "Max matrices (0 = unlimited)")
	cmd.Flags().IntVar(&quotaSessions, "quota-sessions", 0, "Max sessions (0 = unlimited)")
	return cmd
}

func newTeamListCmd(teams *state.TeamStore) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all teams",
		RunE: func(_ *cobra.Command, _ []string) error {
			list, err := (*teams).List()
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Println("No teams found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tDISPLAY NAME\tMEMBERS\tMAX SANDBOXES\tMAX CPU\tMAX MEMORY\tAGE")
			for _, t := range list {
				age := time.Since(t.CreatedAt).Truncate(time.Second)
				maxSb := "unlimited"
				if t.Quota.MaxSandboxes > 0 {
					maxSb = fmt.Sprintf("%d", t.Quota.MaxSandboxes)
				}
				maxCPU := "unlimited"
				if t.Quota.MaxCPU != "" {
					maxCPU = t.Quota.MaxCPU
				}
				maxMem := "unlimited"
				if t.Quota.MaxMemory != "" {
					maxMem = t.Quota.MaxMemory
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
					t.Name,
					t.DisplayName,
					len(t.Members),
					maxSb,
					maxCPU,
					maxMem,
					age,
				)
			}
			return w.Flush()
		},
	}
}

func newTeamInspectCmd(teams *state.TeamStore) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <name>",
		Short: "Show detailed team information and current usage",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			team, err := (*teams).Get(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Name:         %s\n", team.Name)
			if team.DisplayName != "" {
				fmt.Printf("Display Name: %s\n", team.DisplayName)
			}
			fmt.Printf("Created:      %s\n", team.CreatedAt.Format(time.RFC3339))
			fmt.Printf("Members:      %d\n", len(team.Members))
			for _, m := range team.Members {
				fmt.Printf("  - %s (%s)\n", m.UserName, m.Role)
			}

			fmt.Printf("Quota:\n")
			if team.Quota.MaxSandboxes > 0 {
				fmt.Printf("  Max Sandboxes: %d\n", team.Quota.MaxSandboxes)
			}
			if team.Quota.MaxCPU != "" {
				fmt.Printf("  Max CPU:       %s\n", team.Quota.MaxCPU)
			}
			if team.Quota.MaxMemory != "" {
				fmt.Printf("  Max Memory:    %s\n", team.Quota.MaxMemory)
			}
			if team.Quota.MaxDisk != "" {
				fmt.Printf("  Max Disk:      %s\n", team.Quota.MaxDisk)
			}
			if team.Quota.MaxGPUs > 0 {
				fmt.Printf("  Max GPUs:      %d\n", team.Quota.MaxGPUs)
			}
			if team.Quota.MaxMatrices > 0 {
				fmt.Printf("  Max Matrices:  %d\n", team.Quota.MaxMatrices)
			}
			if team.Quota.MaxSessions > 0 {
				fmt.Printf("  Max Sessions:  %d\n", team.Quota.MaxSessions)
			}

			// Try to compute usage if sandbox store is available.
			sandboxStore, err := state.NewFileStore()
			if err == nil {
				matrixStore, _ := state.NewFileMatrixStore()
				sessionStore, _ := state.NewFileSessionStore()
				qm := quota.New(*teams, sandboxStore, matrixStore, sessionStore)
				usage, err := qm.GetUsage(team.Name)
				if err == nil {
					fmt.Printf("Usage:\n")
					fmt.Printf("  Sandboxes: %d\n", usage.Sandboxes)
					fmt.Printf("  CPU:       %.2f cores\n", usage.CPUCores)
					fmt.Printf("  Memory:    %s\n", formatBytesHuman(usage.MemoryBytes))
					fmt.Printf("  GPUs:      %d\n", usage.GPUs)
					fmt.Printf("  Matrices:  %d\n", usage.Matrices)
					fmt.Printf("  Sessions:  %d\n", usage.Sessions)
				}
			}

			return nil
		},
	}
}

func newTeamAddMemberCmd(teams *state.TeamStore) *cobra.Command {
	var role string

	cmd := &cobra.Command{
		Use:   "add-member <team> <user>",
		Short: "Add a member to a team",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			teamName := args[0]
			userName := args[1]

			team, err := (*teams).Get(teamName)
			if err != nil {
				return err
			}

			// Check if member already exists.
			for _, m := range team.Members {
				if m.UserName == userName {
					return fmt.Errorf("user %q is already a member of team %q", userName, teamName)
				}
			}

			team.Members = append(team.Members, v1alpha1.TeamMember{
				UserName: userName,
				Role:     v1alpha1.Role(role),
			})

			if err := (*teams).Save(team); err != nil {
				return err
			}
			fmt.Printf("Added %q to team %q with role %q.\n", userName, teamName, role)
			return nil
		},
	}

	cmd.Flags().StringVar(&role, "role", "operator", "Role for the member (admin, operator, viewer)")
	return cmd
}

func newTeamRemoveMemberCmd(teams *state.TeamStore) *cobra.Command {
	return &cobra.Command{
		Use:   "remove-member <team> <user>",
		Short: "Remove a member from a team",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			teamName := args[0]
			userName := args[1]

			team, err := (*teams).Get(teamName)
			if err != nil {
				return err
			}

			found := false
			members := make([]v1alpha1.TeamMember, 0, len(team.Members))
			for _, m := range team.Members {
				if m.UserName == userName {
					found = true
					continue
				}
				members = append(members, m)
			}

			if !found {
				return fmt.Errorf("user %q is not a member of team %q", userName, teamName)
			}

			team.Members = members
			if err := (*teams).Save(team); err != nil {
				return err
			}
			fmt.Printf("Removed %q from team %q.\n", userName, teamName)
			return nil
		},
	}
}

func newTeamSetQuotaCmd(teams *state.TeamStore) *cobra.Command {
	var (
		sandboxes int
		cpu       string
		memory    string
		disk      string
		gpus      int
		matrices  int
		sessions  int
	)

	cmd := &cobra.Command{
		Use:   "set-quota <team>",
		Short: "Update resource quotas for a team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			teamName := args[0]

			team, err := (*teams).Get(teamName)
			if err != nil {
				return err
			}

			if cmd.Flags().Changed("sandboxes") {
				team.Quota.MaxSandboxes = sandboxes
			}
			if cmd.Flags().Changed("cpu") {
				team.Quota.MaxCPU = cpu
			}
			if cmd.Flags().Changed("memory") {
				team.Quota.MaxMemory = memory
			}
			if cmd.Flags().Changed("disk") {
				team.Quota.MaxDisk = disk
			}
			if cmd.Flags().Changed("gpus") {
				team.Quota.MaxGPUs = gpus
			}
			if cmd.Flags().Changed("matrices") {
				team.Quota.MaxMatrices = matrices
			}
			if cmd.Flags().Changed("sessions") {
				team.Quota.MaxSessions = sessions
			}

			if err := (*teams).Save(team); err != nil {
				return err
			}
			fmt.Printf("Quotas updated for team %q.\n", teamName)
			return nil
		},
	}

	cmd.Flags().IntVar(&sandboxes, "sandboxes", 0, "Max sandboxes (0 = unlimited)")
	cmd.Flags().StringVar(&cpu, "cpu", "", "Max CPU cores")
	cmd.Flags().StringVar(&memory, "memory", "", "Max memory (e.g. \"16G\")")
	cmd.Flags().StringVar(&disk, "disk", "", "Max disk (e.g. \"100G\")")
	cmd.Flags().IntVar(&gpus, "gpus", 0, "Max GPUs (0 = unlimited)")
	cmd.Flags().IntVar(&matrices, "matrices", 0, "Max matrices (0 = unlimited)")
	cmd.Flags().IntVar(&sessions, "sessions", 0, "Max sessions (0 = unlimited)")
	return cmd
}

// formatBytesHuman formats bytes into a human-readable string.
func formatBytesHuman(b int64) string {
	const (
		gb = 1_000_000_000
		mb = 1_000_000
		kb = 1_000
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fG", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1fM", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1fK", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

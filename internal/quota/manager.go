// Package quota provides resource quota management for teams.
package quota

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// QuotaRequest describes the resources being requested.
type QuotaRequest struct {
	Sandboxes int
	CPU       string // e.g. "2", "0.5"
	Memory    string // e.g. "4G", "512M"
	Disk      string // e.g. "10G"
	GPUs      int
	Matrices  int
	Sessions  int
}

// QuotaExceededError is returned when a resource quota would be exceeded.
type QuotaExceededError struct {
	Team     string
	Resource string
	Limit    string
	Current  string
	Request  string
}

func (e *QuotaExceededError) Error() string {
	return fmt.Sprintf("quota exceeded for team %q: %s limit is %s (current: %s, requested: %s)",
		e.Team, e.Resource, e.Limit, e.Current, e.Request)
}

// Manager validates resource quotas for teams.
type Manager struct {
	teams     state.TeamStore
	sandboxes state.Store
	matrices  state.MatrixStore
	sessions  state.SessionStore
}

// New creates a new quota Manager.
func New(teams state.TeamStore, sandboxes state.Store, matrices state.MatrixStore, sessions state.SessionStore) *Manager {
	return &Manager{
		teams:     teams,
		sandboxes: sandboxes,
		matrices:  matrices,
		sessions:  sessions,
	}
}

// CheckQuota validates that the requested resources do not exceed the team's
// quota. It returns nil if the request is within limits, or a
// QuotaExceededError with details about which limit was exceeded.
func (m *Manager) CheckQuota(_ context.Context, teamName string, request QuotaRequest) error {
	team, err := m.teams.Get(teamName)
	if err != nil {
		return fmt.Errorf("get team %q: %w", teamName, err)
	}

	usage, err := m.GetUsage(teamName)
	if err != nil {
		return fmt.Errorf("get usage for team %q: %w", teamName, err)
	}

	// Check sandbox count.
	if team.Quota.MaxSandboxes > 0 && request.Sandboxes > 0 {
		if usage.Sandboxes+request.Sandboxes > team.Quota.MaxSandboxes {
			return &QuotaExceededError{
				Team:     teamName,
				Resource: "sandboxes",
				Limit:    strconv.Itoa(team.Quota.MaxSandboxes),
				Current:  strconv.Itoa(usage.Sandboxes),
				Request:  strconv.Itoa(request.Sandboxes),
			}
		}
	}

	// Check CPU.
	if team.Quota.MaxCPU != "" && request.CPU != "" {
		maxCPU, err := parseCPU(team.Quota.MaxCPU)
		if err != nil {
			return fmt.Errorf("parse team max CPU %q: %w", team.Quota.MaxCPU, err)
		}
		reqCPU, err := parseCPU(request.CPU)
		if err != nil {
			return fmt.Errorf("parse requested CPU %q: %w", request.CPU, err)
		}
		if usage.CPUCores+reqCPU > maxCPU {
			return &QuotaExceededError{
				Team:     teamName,
				Resource: "cpu",
				Limit:    team.Quota.MaxCPU,
				Current:  strconv.FormatFloat(usage.CPUCores, 'f', 2, 64),
				Request:  request.CPU,
			}
		}
	}

	// Check memory.
	if team.Quota.MaxMemory != "" && request.Memory != "" {
		maxMem, err := ParseResourceValue(team.Quota.MaxMemory)
		if err != nil {
			return fmt.Errorf("parse team max memory %q: %w", team.Quota.MaxMemory, err)
		}
		reqMem, err := ParseResourceValue(request.Memory)
		if err != nil {
			return fmt.Errorf("parse requested memory %q: %w", request.Memory, err)
		}
		if usage.MemoryBytes+reqMem > maxMem {
			return &QuotaExceededError{
				Team:     teamName,
				Resource: "memory",
				Limit:    team.Quota.MaxMemory,
				Current:  formatBytes(usage.MemoryBytes),
				Request:  request.Memory,
			}
		}
	}

	// Check disk.
	if team.Quota.MaxDisk != "" && request.Disk != "" {
		maxDisk, err := ParseResourceValue(team.Quota.MaxDisk)
		if err != nil {
			return fmt.Errorf("parse team max disk %q: %w", team.Quota.MaxDisk, err)
		}
		reqDisk, err := ParseResourceValue(request.Disk)
		if err != nil {
			return fmt.Errorf("parse requested disk %q: %w", request.Disk, err)
		}
		if usage.DiskBytes+reqDisk > maxDisk {
			return &QuotaExceededError{
				Team:     teamName,
				Resource: "disk",
				Limit:    team.Quota.MaxDisk,
				Current:  formatBytes(usage.DiskBytes),
				Request:  request.Disk,
			}
		}
	}

	// Check GPU count.
	if team.Quota.MaxGPUs > 0 && request.GPUs > 0 {
		if usage.GPUs+request.GPUs > team.Quota.MaxGPUs {
			return &QuotaExceededError{
				Team:     teamName,
				Resource: "gpus",
				Limit:    strconv.Itoa(team.Quota.MaxGPUs),
				Current:  strconv.Itoa(usage.GPUs),
				Request:  strconv.Itoa(request.GPUs),
			}
		}
	}

	// Check matrix count.
	if team.Quota.MaxMatrices > 0 && request.Matrices > 0 {
		if usage.Matrices+request.Matrices > team.Quota.MaxMatrices {
			return &QuotaExceededError{
				Team:     teamName,
				Resource: "matrices",
				Limit:    strconv.Itoa(team.Quota.MaxMatrices),
				Current:  strconv.Itoa(usage.Matrices),
				Request:  strconv.Itoa(request.Matrices),
			}
		}
	}

	// Check session count.
	if team.Quota.MaxSessions > 0 && request.Sessions > 0 {
		if usage.Sessions+request.Sessions > team.Quota.MaxSessions {
			return &QuotaExceededError{
				Team:     teamName,
				Resource: "sessions",
				Limit:    strconv.Itoa(team.Quota.MaxSessions),
				Current:  strconv.Itoa(usage.Sessions),
				Request:  strconv.Itoa(request.Sessions),
			}
		}
	}

	return nil
}

// GetUsage calculates the current resource usage for a team by scanning all
// sandboxes, matrices, and sessions that belong to that team.
func (m *Manager) GetUsage(teamName string) (*v1alpha1.ResourceUsage, error) {
	usage := &v1alpha1.ResourceUsage{}

	// Count sandboxes and their resources.
	sandboxes, err := m.sandboxes.List()
	if err != nil {
		return nil, fmt.Errorf("list sandboxes: %w", err)
	}
	for _, sb := range sandboxes {
		if sb.Spec.Team != teamName {
			continue
		}
		usage.Sandboxes++

		// Accumulate CPU.
		if sb.Spec.Resources.CPU != "" {
			cpu, err := parseCPU(sb.Spec.Resources.CPU)
			if err == nil {
				usage.CPUCores += cpu
			}
		}

		// Accumulate memory.
		if sb.Spec.Resources.Memory != "" {
			mem, err := ParseResourceValue(sb.Spec.Resources.Memory)
			if err == nil {
				usage.MemoryBytes += mem
			}
		}

		// Accumulate disk.
		if sb.Spec.Resources.Disk != "" {
			disk, err := ParseResourceValue(sb.Spec.Resources.Disk)
			if err == nil {
				usage.DiskBytes += disk
			}
		}

		// Accumulate GPUs.
		if sb.Spec.Resources.GPU != nil {
			usage.GPUs += sb.Spec.Resources.GPU.Count
		}
	}

	// Count matrices.
	if m.matrices != nil {
		matrices, err := m.matrices.List()
		if err != nil {
			return nil, fmt.Errorf("list matrices: %w", err)
		}
		for _, mx := range matrices {
			if mx.Spec.Team == teamName {
				usage.Matrices++
			}
		}
	}

	// Count active sessions for team sandboxes.
	if m.sessions != nil {
		sessions, err := m.sessions.ListSessions()
		if err != nil {
			return nil, fmt.Errorf("list sessions: %w", err)
		}
		// Build a set of team sandbox names for lookup.
		teamSandboxes := make(map[string]bool)
		for _, sb := range sandboxes {
			if sb.Spec.Team == teamName {
				teamSandboxes[sb.Metadata.Name] = true
			}
		}
		for _, s := range sessions {
			if s.State == v1alpha1.SessionStateActive && teamSandboxes[s.Sandbox] {
				usage.Sessions++
			}
		}
	}

	return usage, nil
}

// ParseResourceValue parses a resource value string like "2G", "512M", "1T"
// into bytes. It also handles plain numeric values (interpreted as bytes).
func ParseResourceValue(val string) (int64, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0, fmt.Errorf("empty resource value")
	}

	// Try to parse as a plain number (bytes).
	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		return n, nil
	}

	// Try to parse as a float (bytes).
	if n, err := strconv.ParseFloat(val, 64); err == nil {
		return int64(n), nil
	}

	// Parse suffix.
	upper := strings.ToUpper(val)
	var multiplier int64
	var numStr string

	switch {
	case strings.HasSuffix(upper, "TI"):
		multiplier = 1 << 40
		numStr = val[:len(val)-2]
	case strings.HasSuffix(upper, "GI"):
		multiplier = 1 << 30
		numStr = val[:len(val)-2]
	case strings.HasSuffix(upper, "MI"):
		multiplier = 1 << 20
		numStr = val[:len(val)-2]
	case strings.HasSuffix(upper, "KI"):
		multiplier = 1 << 10
		numStr = val[:len(val)-2]
	case strings.HasSuffix(upper, "T"):
		multiplier = 1_000_000_000_000
		numStr = val[:len(val)-1]
	case strings.HasSuffix(upper, "G"):
		multiplier = 1_000_000_000
		numStr = val[:len(val)-1]
	case strings.HasSuffix(upper, "M"):
		multiplier = 1_000_000
		numStr = val[:len(val)-1]
	case strings.HasSuffix(upper, "K"):
		multiplier = 1_000
		numStr = val[:len(val)-1]
	default:
		return 0, fmt.Errorf("unrecognized resource value %q", val)
	}

	n, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("parse numeric part of %q: %w", val, err)
	}

	return int64(math.Round(n * float64(multiplier))), nil
}

// parseCPU parses a CPU value string like "4", "0.5", "2" into a float64
// representing CPU cores.
func parseCPU(val string) (float64, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0, fmt.Errorf("empty CPU value")
	}
	n, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, fmt.Errorf("parse CPU value %q: %w", val, err)
	}
	return n, nil
}

// formatBytes formats a byte count into a human-readable string.
func formatBytes(b int64) string {
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
		return fmt.Sprintf("%d", b)
	}
}

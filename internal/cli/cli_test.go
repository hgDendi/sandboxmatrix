package cli

import (
	"testing"
)

func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()
	if cmd.Use != "smx" {
		t.Errorf("expected Use=smx, got %q", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("expected non-empty Short description")
	}

	// Verify subcommands are registered.
	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Use] = true
	}

	for _, expected := range []string{"version", "blueprint", "sandbox", "session", "matrix", "pool"} {
		if !subcommands[expected] {
			t.Errorf("expected subcommand %q, got %v", expected, subcommands)
		}
	}
}

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()
	if cmd.Use != "version" {
		t.Errorf("expected Use=version, got %q", cmd.Use)
	}

	// Run the version command -- just verify it executes without error.
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command: %v", err)
	}
}

func TestNewVersionCmdJSON(t *testing.T) {
	cmd := newVersionCmd()
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version --json: %v", err)
	}
}

func TestParseMembers(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantLen int
		wantErr bool
	}{
		{
			name:    "valid members",
			input:   []string{"worker1:bp1.yaml", "worker2:bp2.yaml"},
			wantLen: 2,
		},
		{
			name:    "single member",
			input:   []string{"w:blueprint.yaml"},
			wantLen: 1,
		},
		{
			name:    "empty input",
			input:   []string{},
			wantLen: 0,
		},
		{
			name:    "missing colon",
			input:   []string{"invalid"},
			wantErr: true,
		},
		{
			name:    "empty name",
			input:   []string{":blueprint.yaml"},
			wantErr: true,
		},
		{
			name:    "empty blueprint",
			input:   []string{"worker:"},
			wantErr: true,
		},
		{
			name:    "colon in blueprint path",
			input:   []string{"worker:path/to/bp.yaml"},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			members, err := parseMembers(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(members) != tt.wantLen {
				t.Errorf("expected %d members, got %d", tt.wantLen, len(members))
			}
		})
	}
}

func TestParseMembersValues(t *testing.T) {
	members, err := parseMembers([]string{"alpha:blueprints/python.yaml"})
	if err != nil {
		t.Fatalf("parseMembers: %v", err)
	}
	if members[0].Name != "alpha" {
		t.Errorf("expected name alpha, got %q", members[0].Name)
	}
	if members[0].Blueprint != "blueprints/python.yaml" {
		t.Errorf("expected blueprint path, got %q", members[0].Blueprint)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "unix newlines",
			input: "line1\nline2\nline3",
			want:  []string{"line1", "line2", "line3"},
		},
		{
			name:  "windows newlines",
			input: "line1\r\nline2\r\nline3",
			want:  []string{"line1", "line2", "line3"},
		},
		{
			name:  "single line",
			input: "hello",
			want:  []string{"hello"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "trailing newline",
			input: "line1\nline2\n",
			want:  []string{"line1", "line2"},
		},
		{
			name:  "mixed newlines",
			input: "a\nb\r\nc",
			want:  []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len: got %d, want %d (%v vs %v)", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNewBlueprintCmd(t *testing.T) {
	cmd := newBlueprintCmd()
	if cmd.Use != "blueprint" {
		t.Errorf("expected Use=blueprint, got %q", cmd.Use)
	}
	// Should have validate and inspect subcommands.
	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}
	if !subcommands["validate"] {
		t.Error("expected validate subcommand")
	}
	if !subcommands["inspect"] {
		t.Error("expected inspect subcommand")
	}
}

func TestNewAuthCmd(t *testing.T) {
	cmd := newAuthCmd()
	if cmd.Use != "auth" {
		t.Errorf("expected Use=auth, got %q", cmd.Use)
	}
	// Should have add-user, list-users, remove-user, audit subcommands.
	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}
	for _, expected := range []string{"add-user", "list-users", "remove-user", "audit"} {
		if !subcommands[expected] {
			t.Errorf("expected subcommand %q", expected)
		}
	}
}

func TestNewConfigCmd(t *testing.T) {
	cmd := newConfigCmd()
	if cmd.Use != "config" {
		t.Errorf("expected Use=config, got %q", cmd.Use)
	}
}

func TestNewServerCmd(t *testing.T) {
	cmd := newServerCmd()
	if cmd.Use != "server" {
		t.Errorf("expected Use=server, got %q", cmd.Use)
	}
}

func TestNewDashboardCmd(t *testing.T) {
	cmd := newDashboardCmd()
	if cmd.Use != "dashboard" {
		t.Errorf("expected Use=dashboard, got %q", cmd.Use)
	}
}

func TestNewOperatorCmd(t *testing.T) {
	cmd := newOperatorCmd()
	if cmd.Use != "operator" {
		t.Errorf("expected Use=operator, got %q", cmd.Use)
	}
}

func TestNewMCPCmd(t *testing.T) {
	cmd := newMCPCmd()
	if cmd.Use != "mcp" {
		t.Errorf("expected Use=mcp, got %q", cmd.Use)
	}
}

func TestNewA2ACmd(t *testing.T) {
	cmd := newA2ACmd()
	if cmd.Use != "a2a" {
		t.Errorf("expected Use=a2a, got %q", cmd.Use)
	}
}

func TestNewPoolCmd(t *testing.T) {
	cmd := newPoolCmd()
	if cmd.Use != "pool" {
		t.Errorf("expected Use=pool, got %q", cmd.Use)
	}
}

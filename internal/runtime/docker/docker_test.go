package docker

import (
	"testing"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
)

func TestParseMemory(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{
			name:  "gigabytes integer",
			input: "2Gi",
			want:  2 * 1024 * 1024 * 1024,
		},
		{
			name:  "gigabytes fractional",
			input: "1.5Gi",
			want:  int64(1.5 * 1024 * 1024 * 1024),
		},
		{
			name:  "megabytes integer",
			input: "512Mi",
			want:  512 * 1024 * 1024,
		},
		{
			name:  "megabytes fractional",
			input: "256.5Mi",
			want:  int64(256.5 * 1024 * 1024),
		},
		{
			name:  "raw bytes",
			input: "1073741824",
			want:  1073741824,
		},
		{
			name:  "raw bytes small",
			input: "0",
			want:  0,
		},
		{
			name:  "whitespace trimmed",
			input: "  4Gi  ",
			want:  4 * 1024 * 1024 * 1024,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid suffix",
			input:   "100XX",
			wantErr: true,
		},
		{
			name:    "invalid Gi value",
			input:   "abcGi",
			wantErr: true,
		},
		{
			name:    "invalid Mi value",
			input:   "xyzMi",
			wantErr: true,
		},
		{
			name:    "only letters",
			input:   "hello",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemory(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseMemory(%q) expected error, got %d", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMemory(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseMemory(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSafePrefix(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{
			name: "normal truncation",
			s:    "abcdefghijklmnop",
			n:    5,
			want: "abcde",
		},
		{
			name: "shorter than n",
			s:    "abc",
			n:    10,
			want: "abc",
		},
		{
			name: "empty string",
			s:    "",
			n:    5,
			want: "",
		},
		{
			name: "exact length",
			s:    "abcde",
			n:    5,
			want: "abcde",
		},
		{
			name: "n is zero",
			s:    "abc",
			n:    0,
			want: "",
		},
		{
			name: "n is one",
			s:    "hello",
			n:    1,
			want: "h",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safePrefix(tt.s, tt.n)
			if got != tt.want {
				t.Errorf("safePrefix(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

func TestFormatBind(t *testing.T) {
	tests := []struct {
		name  string
		mount runtime.Mount
		want  string
	}{
		{
			name: "normal read-write mount",
			mount: runtime.Mount{
				Source: "/host/data",
				Target: "/container/data",
			},
			want: "/host/data:/container/data",
		},
		{
			name: "read-only mount",
			mount: runtime.Mount{
				Source:   "/host/config",
				Target:   "/container/config",
				ReadOnly: true,
			},
			want: "/host/config:/container/config:ro",
		},
		{
			name: "minimal mount paths",
			mount: runtime.Mount{
				Source: "/a",
				Target: "/b",
			},
			want: "/a:/b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBind(tt.mount)
			if got != tt.want {
				t.Errorf("formatBind(%+v) = %q, want %q", tt.mount, got, tt.want)
			}
		})
	}
}

func TestBuildLabels(t *testing.T) {
	t.Run("with custom labels", func(t *testing.T) {
		r := &Runtime{}
		cfg := &runtime.CreateConfig{
			Name: "test-sandbox",
			Labels: map[string]string{
				"env":     "staging",
				"project": "demo",
			},
		}

		labels := r.buildLabels(cfg)

		// Check managed label is always set.
		if labels[labelManaged] != "true" {
			t.Errorf("expected label %q = %q, got %q", labelManaged, "true", labels[labelManaged])
		}

		// Check name label is always set.
		if labels[labelName] != "test-sandbox" {
			t.Errorf("expected label %q = %q, got %q", labelName, "test-sandbox", labels[labelName])
		}

		// Check custom labels are preserved.
		if labels["env"] != "staging" {
			t.Errorf("expected custom label env = %q, got %q", "staging", labels["env"])
		}
		if labels["project"] != "demo" {
			t.Errorf("expected custom label project = %q, got %q", "demo", labels["project"])
		}
	})

	t.Run("without custom labels", func(t *testing.T) {
		r := &Runtime{}
		cfg := &runtime.CreateConfig{
			Name: "empty-labels",
		}

		labels := r.buildLabels(cfg)

		if labels[labelManaged] != "true" {
			t.Errorf("expected label %q = %q, got %q", labelManaged, "true", labels[labelManaged])
		}
		if labels[labelName] != "empty-labels" {
			t.Errorf("expected label %q = %q, got %q", labelName, "empty-labels", labels[labelName])
		}
		// Should only have the two mandatory labels.
		if len(labels) != 2 {
			t.Errorf("expected 2 labels, got %d: %v", len(labels), labels)
		}
	})

	t.Run("custom labels do not override managed labels", func(t *testing.T) {
		r := &Runtime{}
		cfg := &runtime.CreateConfig{
			Name: "override-test",
			Labels: map[string]string{
				labelManaged: "false",
				labelName:    "sneaky",
			},
		}

		labels := r.buildLabels(cfg)

		// The buildLabels function sets managed labels after copying custom
		// labels, so the managed values must win.
		if labels[labelManaged] != "true" {
			t.Errorf("managed label should be 'true', got %q", labels[labelManaged])
		}
		if labels[labelName] != "override-test" {
			t.Errorf("name label should be 'override-test', got %q", labels[labelName])
		}
	})
}

func TestWithOCIRuntime(t *testing.T) {
	t.Run("sets ociRuntime field", func(t *testing.T) {
		r := &Runtime{}
		opt := WithOCIRuntime("runsc")
		opt(r)

		if r.ociRuntime != "runsc" {
			t.Errorf("expected ociRuntime = %q, got %q", "runsc", r.ociRuntime)
		}
	})

	t.Run("empty string clears ociRuntime", func(t *testing.T) {
		r := &Runtime{ociRuntime: "runsc"}
		opt := WithOCIRuntime("")
		opt(r)

		if r.ociRuntime != "" {
			t.Errorf("expected ociRuntime = %q, got %q", "", r.ociRuntime)
		}
	})
}

func TestRuntimeName(t *testing.T) {
	r := &Runtime{}
	got := r.Name()
	if got != "docker" {
		t.Errorf("Runtime.Name() = %q, want %q", got, "docker")
	}
}

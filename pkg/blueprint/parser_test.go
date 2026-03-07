package blueprint

import (
	"testing"
)

func TestParse(t *testing.T) {
	data := []byte(`
apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: test-bp
  version: "1.0.0"
spec:
  base: python:3.12-slim
  runtime: docker
  resources:
    cpu: "2"
    memory: 2Gi
  workspace:
    mountPath: /workspace
  network:
    expose: [8000]
`)

	bp, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if bp.Metadata.Name != "test-bp" {
		t.Errorf("expected name 'test-bp', got %q", bp.Metadata.Name)
	}
	if bp.Spec.Base != "python:3.12-slim" {
		t.Errorf("expected base 'python:3.12-slim', got %q", bp.Spec.Base)
	}
	if bp.Spec.Runtime != "docker" {
		t.Errorf("expected runtime 'docker', got %q", bp.Spec.Runtime)
	}
	if bp.Spec.Resources.CPU != "2" {
		t.Errorf("expected CPU '2', got %q", bp.Spec.Resources.CPU)
	}
	if len(bp.Spec.Network.Expose) != 1 || bp.Spec.Network.Expose[0] != 8000 {
		t.Errorf("expected expose [8000], got %v", bp.Spec.Network.Expose)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantErrs int
	}{
		{
			name: "valid blueprint",
			yaml: `
apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: valid
spec:
  base: ubuntu:24.04
  runtime: docker
`,
			wantErrs: 0,
		},
		{
			name: "missing required fields",
			yaml: `
apiVersion: wrong
kind: Wrong
metadata:
  name: ""
spec:
  base: ""
  runtime: ""
`,
			wantErrs: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bp, err := Parse([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			errs := Validate(bp)
			if len(errs) != tt.wantErrs {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErrs, len(errs), errs)
			}
		})
	}
}

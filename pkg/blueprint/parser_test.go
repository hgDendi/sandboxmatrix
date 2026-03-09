package blueprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
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

func TestParseInvalidYAML(t *testing.T) {
	_, err := Parse([]byte(`{{{invalid yaml`))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "parse blueprint YAML") {
		t.Errorf("expected 'parse blueprint YAML' error, got: %v", err)
	}
}

func TestParseFileNotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/blueprint.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read blueprint file") {
		t.Errorf("expected 'read blueprint file' error, got: %v", err)
	}
}

func TestParseFileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	content := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: from-file
spec:
  base: alpine:latest
  runtime: docker
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	bp, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if bp.Metadata.Name != "from-file" {
		t.Errorf("expected name from-file, got %q", bp.Metadata.Name)
	}
}

func TestValidateFileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.yaml")
	content := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: valid-bp
spec:
  base: alpine:latest
  runtime: docker
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	bp, errs := ValidateFile(path)
	if len(errs) > 0 {
		t.Fatalf("expected no validation errors, got: %v", errs)
	}
	if bp.Metadata.Name != "valid-bp" {
		t.Errorf("expected name valid-bp, got %q", bp.Metadata.Name)
	}
}

func TestValidateFileInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	content := `apiVersion: wrong
kind: Wrong
metadata:
  name: ""
spec:
  base: ""
  runtime: ""
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, errs := ValidateFile(path)
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}
}

func TestValidateFileNotFound(t *testing.T) {
	_, errs := ValidateFile("/nonexistent/blueprint.yaml")
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "read blueprint file") {
		t.Errorf("expected file read error, got: %v", errs[0])
	}
}

func TestValidateWrongAPIVersion(t *testing.T) {
	bp := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "wrong/v1", Kind: "Blueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: "test"},
		Spec:     v1alpha1.BlueprintSpec{Base: "alpine", Runtime: "docker"},
	}
	errs := Validate(bp)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "apiVersion") {
		t.Errorf("expected apiVersion error, got: %v", errs[0])
	}
}

func TestValidateWrongKind(t *testing.T) {
	bp := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "NotBlueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: "test"},
		Spec:     v1alpha1.BlueprintSpec{Base: "alpine", Runtime: "docker"},
	}
	errs := Validate(bp)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "kind") {
		t.Errorf("expected kind error, got: %v", errs[0])
	}
}

func TestValidateMissingName(t *testing.T) {
	bp := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: ""},
		Spec:     v1alpha1.BlueprintSpec{Base: "alpine", Runtime: "docker"},
	}
	errs := Validate(bp)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "metadata.name") {
		t.Errorf("expected metadata.name error, got: %v", errs[0])
	}
}

func TestValidateMissingBase(t *testing.T) {
	bp := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: "test"},
		Spec:     v1alpha1.BlueprintSpec{Base: "", Runtime: "docker"},
	}
	errs := Validate(bp)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "spec.base") {
		t.Errorf("expected spec.base error, got: %v", errs[0])
	}
}

func TestValidateMissingRuntime(t *testing.T) {
	bp := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: "test"},
		Spec:     v1alpha1.BlueprintSpec{Base: "alpine", Runtime: ""},
	}
	errs := Validate(bp)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "spec.runtime") {
		t.Errorf("expected spec.runtime error, got: %v", errs[0])
	}
}

func TestValidateFirecrackerRuntime(t *testing.T) {
	bp := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: "test"},
		Spec:     v1alpha1.BlueprintSpec{Base: "alpine", Runtime: "firecracker"},
	}
	errs := Validate(bp)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "firecracker") {
		t.Errorf("expected firecracker error, got: %v", errs[0])
	}
}

func TestValidateGPU(t *testing.T) {
	t.Run("valid GPU config", func(t *testing.T) {
		bp := &v1alpha1.Blueprint{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
			Metadata: v1alpha1.ObjectMeta{Name: "gpu-bp"},
			Spec: v1alpha1.BlueprintSpec{
				Base:    "nvidia/cuda:latest",
				Runtime: "docker",
				Resources: v1alpha1.Resources{
					GPU: &v1alpha1.GPUSpec{Count: 1, Driver: "nvidia"},
				},
			},
		}
		errs := Validate(bp)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("GPU count zero", func(t *testing.T) {
		bp := &v1alpha1.Blueprint{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
			Metadata: v1alpha1.ObjectMeta{Name: "gpu-bp"},
			Spec: v1alpha1.BlueprintSpec{
				Base:    "nvidia/cuda:latest",
				Runtime: "docker",
				Resources: v1alpha1.Resources{
					GPU: &v1alpha1.GPUSpec{Count: 0},
				},
			},
		}
		errs := Validate(bp)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), "gpu.count") {
			t.Errorf("expected gpu.count error, got: %v", errs[0])
		}
	})

	t.Run("GPU negative count", func(t *testing.T) {
		bp := &v1alpha1.Blueprint{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
			Metadata: v1alpha1.ObjectMeta{Name: "gpu-bp"},
			Spec: v1alpha1.BlueprintSpec{
				Base:    "nvidia/cuda:latest",
				Runtime: "docker",
				Resources: v1alpha1.Resources{
					GPU: &v1alpha1.GPUSpec{Count: -1},
				},
			},
		}
		errs := Validate(bp)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
	})

	t.Run("GPU default driver", func(t *testing.T) {
		bp := &v1alpha1.Blueprint{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
			Metadata: v1alpha1.ObjectMeta{Name: "gpu-bp"},
			Spec: v1alpha1.BlueprintSpec{
				Base:    "nvidia/cuda:latest",
				Runtime: "docker",
				Resources: v1alpha1.Resources{
					GPU: &v1alpha1.GPUSpec{Count: 1, Driver: ""},
				},
			},
		}
		errs := Validate(bp)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
		if bp.Spec.Resources.GPU.Driver != "nvidia" {
			t.Errorf("expected default driver nvidia, got %q", bp.Spec.Resources.GPU.Driver)
		}
	})
}

func TestValidateDevices(t *testing.T) {
	t.Run("valid device", func(t *testing.T) {
		bp := &v1alpha1.Blueprint{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
			Metadata: v1alpha1.ObjectMeta{Name: "dev-bp"},
			Spec: v1alpha1.BlueprintSpec{
				Base:    "alpine",
				Runtime: "docker",
				Devices: []v1alpha1.DeviceMapping{
					{HostPath: "/dev/kvm"},
				},
			},
		}
		errs := Validate(bp)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("missing host path", func(t *testing.T) {
		bp := &v1alpha1.Blueprint{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
			Metadata: v1alpha1.ObjectMeta{Name: "dev-bp"},
			Spec: v1alpha1.BlueprintSpec{
				Base:    "alpine",
				Runtime: "docker",
				Devices: []v1alpha1.DeviceMapping{
					{HostPath: ""},
				},
			},
		}
		errs := Validate(bp)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), "hostPath") {
			t.Errorf("expected hostPath error, got: %v", errs[0])
		}
	})

	t.Run("multiple devices one missing", func(t *testing.T) {
		bp := &v1alpha1.Blueprint{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
			Metadata: v1alpha1.ObjectMeta{Name: "dev-bp"},
			Spec: v1alpha1.BlueprintSpec{
				Base:    "alpine",
				Runtime: "docker",
				Devices: []v1alpha1.DeviceMapping{
					{HostPath: "/dev/kvm"},
					{HostPath: ""},
					{HostPath: "/dev/fuse"},
				},
			},
		}
		errs := Validate(bp)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), "devices[1]") {
			t.Errorf("expected devices[1] error, got: %v", errs[0])
		}
	})
}

func TestValidateReadinessProbe(t *testing.T) {
	base := func() *v1alpha1.Blueprint {
		return &v1alpha1.Blueprint{
			TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
			Metadata: v1alpha1.ObjectMeta{Name: "probe-bp"},
			Spec: v1alpha1.BlueprintSpec{
				Base:    "alpine",
				Runtime: "docker",
			},
		}
	}

	t.Run("exec probe valid", func(t *testing.T) {
		bp := base()
		bp.Spec.ReadinessProbe = &v1alpha1.ProbeConfig{
			Type:    "exec",
			Command: []string{"curl", "-f", "http://localhost:8080"},
		}
		errs := Validate(bp)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("exec probe missing command", func(t *testing.T) {
		bp := base()
		bp.Spec.ReadinessProbe = &v1alpha1.ProbeConfig{
			Type:    "exec",
			Command: []string{},
		}
		errs := Validate(bp)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), "exec probe requires a command") {
			t.Errorf("expected exec command error, got: %v", errs[0])
		}
	})

	t.Run("http probe valid", func(t *testing.T) {
		bp := base()
		bp.Spec.ReadinessProbe = &v1alpha1.ProbeConfig{
			Type: "http",
			Port: 8080,
			Path: "/health",
		}
		errs := Validate(bp)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("http probe missing port", func(t *testing.T) {
		bp := base()
		bp.Spec.ReadinessProbe = &v1alpha1.ProbeConfig{
			Type: "http",
			Port: 0,
		}
		errs := Validate(bp)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), "http probe requires a port") {
			t.Errorf("expected http port error, got: %v", errs[0])
		}
	})

	t.Run("tcp probe valid", func(t *testing.T) {
		bp := base()
		bp.Spec.ReadinessProbe = &v1alpha1.ProbeConfig{
			Type: "tcp",
			Port: 5432,
		}
		errs := Validate(bp)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("tcp probe missing port", func(t *testing.T) {
		bp := base()
		bp.Spec.ReadinessProbe = &v1alpha1.ProbeConfig{
			Type: "tcp",
			Port: 0,
		}
		errs := Validate(bp)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), "tcp probe requires a port") {
			t.Errorf("expected tcp port error, got: %v", errs[0])
		}
	})

	t.Run("missing probe type", func(t *testing.T) {
		bp := base()
		bp.Spec.ReadinessProbe = &v1alpha1.ProbeConfig{
			Type: "",
		}
		errs := Validate(bp)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), "type is required") {
			t.Errorf("expected type required error, got: %v", errs[0])
		}
	})

	t.Run("unknown probe type", func(t *testing.T) {
		bp := base()
		bp.Spec.ReadinessProbe = &v1alpha1.ProbeConfig{
			Type: "grpc",
		}
		errs := Validate(bp)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), "must be exec, http, or tcp") {
			t.Errorf("expected type enum error, got: %v", errs[0])
		}
	})

	t.Run("nil probe is valid", func(t *testing.T) {
		bp := base()
		bp.Spec.ReadinessProbe = nil
		errs := Validate(bp)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
	})
}

func TestValidateEmptySpec(t *testing.T) {
	bp := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: "empty-spec"},
		Spec:     v1alpha1.BlueprintSpec{},
	}
	errs := Validate(bp)
	// Should have errors for: base, runtime
	if len(errs) != 2 {
		t.Errorf("expected 2 errors for empty spec, got %d: %v", len(errs), errs)
	}
}

func TestValidateGvisorRuntime(t *testing.T) {
	bp := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: "gvisor-bp"},
		Spec:     v1alpha1.BlueprintSpec{Base: "alpine", Runtime: "gvisor"},
	}
	errs := Validate(bp)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors for gvisor runtime, got %d: %v", len(errs), errs)
	}
}

func TestParseFullBlueprint(t *testing.T) {
	data := []byte(`
apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: full-bp
  version: "2.0.0"
  labels:
    team: backend
spec:
  base: python:3.12-slim
  runtime: docker
  resources:
    cpu: "4"
    memory: 4Gi
    disk: 20Gi
    gpu:
      count: 2
      driver: nvidia
  setup:
    - run: pip install pytest
    - run: pip install flask
  workspace:
    mountPath: /app
    readOnly: true
  network:
    expose: [8000, 8443]
    policy: bridge
    allowDNS: true
  devices:
    - hostPath: /dev/kvm
    - hostPath: /dev/fuse
      containerPath: /dev/fuse
      permissions: rw
  readinessProbe:
    type: http
    port: 8000
    path: /health
    initialDelaySec: 5
    periodSec: 2
    timeoutSec: 10
    successThreshold: 1
    failureThreshold: 3
`)
	bp, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if bp.Metadata.Name != "full-bp" {
		t.Errorf("name: got %q", bp.Metadata.Name)
	}
	if bp.Metadata.Version != "2.0.0" {
		t.Errorf("version: got %q", bp.Metadata.Version)
	}
	if bp.Spec.Resources.CPU != "4" {
		t.Errorf("CPU: got %q", bp.Spec.Resources.CPU)
	}
	if bp.Spec.Resources.GPU == nil {
		t.Fatal("GPU should not be nil")
	}
	if bp.Spec.Resources.GPU.Count != 2 {
		t.Errorf("GPU count: got %d", bp.Spec.Resources.GPU.Count)
	}
	if len(bp.Spec.Setup) != 2 {
		t.Errorf("setup steps: got %d", len(bp.Spec.Setup))
	}
	if bp.Spec.Workspace.MountPath != "/app" {
		t.Errorf("workspace mount: got %q", bp.Spec.Workspace.MountPath)
	}
	if !bp.Spec.Workspace.ReadOnly {
		t.Error("expected workspace readOnly=true")
	}
	if len(bp.Spec.Network.Expose) != 2 {
		t.Errorf("network expose: got %v", bp.Spec.Network.Expose)
	}
	if bp.Spec.Network.Policy != v1alpha1.NetworkPolicyBridge {
		t.Errorf("network policy: got %q", bp.Spec.Network.Policy)
	}
	if !bp.Spec.Network.AllowDNS {
		t.Error("expected allowDNS=true")
	}
	if len(bp.Spec.Devices) != 2 {
		t.Errorf("devices: got %d", len(bp.Spec.Devices))
	}
	if bp.Spec.ReadinessProbe == nil {
		t.Fatal("readinessProbe should not be nil")
	}
	if bp.Spec.ReadinessProbe.Type != "http" {
		t.Errorf("probe type: got %q", bp.Spec.ReadinessProbe.Type)
	}
	if bp.Spec.ReadinessProbe.Port != 8000 {
		t.Errorf("probe port: got %d", bp.Spec.ReadinessProbe.Port)
	}
	if bp.Spec.ReadinessProbe.FailureThreshold != 3 {
		t.Errorf("probe failureThreshold: got %d", bp.Spec.ReadinessProbe.FailureThreshold)
	}

	// Also validate - should pass.
	errs := Validate(bp)
	if len(errs) != 0 {
		t.Errorf("expected 0 validation errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	bp := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "wrong", Kind: "Wrong"},
		Metadata: v1alpha1.ObjectMeta{Name: ""},
		Spec: v1alpha1.BlueprintSpec{
			Base:    "",
			Runtime: "",
			Devices: []v1alpha1.DeviceMapping{
				{HostPath: ""},
			},
			ReadinessProbe: &v1alpha1.ProbeConfig{
				Type: "exec",
			},
		},
	}
	errs := Validate(bp)
	// apiVersion + kind + name + base + runtime + device hostPath + probe command = 7
	if len(errs) != 7 {
		t.Errorf("expected 7 errors, got %d: %v", len(errs), errs)
	}
}

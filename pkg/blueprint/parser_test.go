package blueprint

import (
	"fmt"
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

func TestResolveExtends(t *testing.T) {
	dir := t.TempDir()

	parentContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: parent-bp
  version: "1.0"
spec:
  base: python:3.12-slim
  runtime: docker
  resources:
    cpu: "1"
    memory: 512Mi
  env:
    PYTHONDONTWRITEBYTECODE: "1"
    PIP_NO_CACHE_DIR: "1"
  setup:
    - run: pip install --upgrade pip
  workspace:
    mountPath: /workspace
  network:
    policy: bridge
`
	childContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: child-bp
  version: "2.0"
spec:
  extends: parent.yaml
  resources:
    memory: 2Gi
  setup:
    - run: pip install pandas numpy
  env:
    JUPYTER_PORT: "8888"
  network:
    policy: bridge
    expose: [8888]
`

	parentPath := filepath.Join(dir, "parent.yaml")
	childPath := filepath.Join(dir, "child.yaml")

	if err := os.WriteFile(parentPath, []byte(parentContent), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	if err := os.WriteFile(childPath, []byte(childContent), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	bp, err := ParseFile(childPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Metadata should come from child.
	if bp.Metadata.Name != "child-bp" {
		t.Errorf("expected name 'child-bp', got %q", bp.Metadata.Name)
	}
	if bp.Metadata.Version != "2.0" {
		t.Errorf("expected version '2.0', got %q", bp.Metadata.Version)
	}

	// Base should be inherited from parent (child didn't set it).
	if bp.Spec.Base != "python:3.12-slim" {
		t.Errorf("expected base 'python:3.12-slim', got %q", bp.Spec.Base)
	}

	// Runtime should be inherited from parent.
	if bp.Spec.Runtime != "docker" {
		t.Errorf("expected runtime 'docker', got %q", bp.Spec.Runtime)
	}

	// CPU should be inherited from parent.
	if bp.Spec.Resources.CPU != "1" {
		t.Errorf("expected CPU '1', got %q", bp.Spec.Resources.CPU)
	}

	// Memory should be overridden by child.
	if bp.Spec.Resources.Memory != "2Gi" {
		t.Errorf("expected memory '2Gi', got %q", bp.Spec.Resources.Memory)
	}

	// Setup should be parent + child (appended).
	if len(bp.Spec.Setup) != 2 {
		t.Fatalf("expected 2 setup steps, got %d", len(bp.Spec.Setup))
	}
	if bp.Spec.Setup[0].Run != "pip install --upgrade pip" {
		t.Errorf("expected first setup 'pip install --upgrade pip', got %q", bp.Spec.Setup[0].Run)
	}
	if bp.Spec.Setup[1].Run != "pip install pandas numpy" {
		t.Errorf("expected second setup 'pip install pandas numpy', got %q", bp.Spec.Setup[1].Run)
	}

	// Env should be merged (child wins on conflict).
	if bp.Spec.Env["PYTHONDONTWRITEBYTECODE"] != "1" {
		t.Errorf("expected PYTHONDONTWRITEBYTECODE=1, got %q", bp.Spec.Env["PYTHONDONTWRITEBYTECODE"])
	}
	if bp.Spec.Env["PIP_NO_CACHE_DIR"] != "1" {
		t.Errorf("expected PIP_NO_CACHE_DIR=1, got %q", bp.Spec.Env["PIP_NO_CACHE_DIR"])
	}
	if bp.Spec.Env["JUPYTER_PORT"] != "8888" {
		t.Errorf("expected JUPYTER_PORT=8888, got %q", bp.Spec.Env["JUPYTER_PORT"])
	}

	// Network should come from child (child overrides completely).
	if len(bp.Spec.Network.Expose) != 1 || bp.Spec.Network.Expose[0] != 8888 {
		t.Errorf("expected expose [8888], got %v", bp.Spec.Network.Expose)
	}

	// Extends should be cleared on the result.
	if bp.Spec.Extends != "" {
		t.Errorf("expected extends to be cleared, got %q", bp.Spec.Extends)
	}
}

func TestResolveExtendsWorkspaceInherited(t *testing.T) {
	dir := t.TempDir()

	parentContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: parent-bp
spec:
  base: alpine
  runtime: docker
  workspace:
    mountPath: /workspace
    readOnly: true
`
	childContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: child-bp
spec:
  extends: parent.yaml
`

	if err := os.WriteFile(filepath.Join(dir, "parent.yaml"), []byte(parentContent), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child.yaml"), []byte(childContent), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	bp, err := ParseFile(filepath.Join(dir, "child.yaml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Workspace should be inherited from parent since child didn't set it.
	if bp.Spec.Workspace.MountPath != "/workspace" {
		t.Errorf("expected workspace mountPath '/workspace', got %q", bp.Spec.Workspace.MountPath)
	}
	if !bp.Spec.Workspace.ReadOnly {
		t.Error("expected workspace readOnly=true from parent")
	}
}

func TestResolveExtendsChildOverridesBase(t *testing.T) {
	dir := t.TempDir()

	parentContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: parent-bp
spec:
  base: python:3.12-slim
  runtime: docker
`
	childContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: child-bp
spec:
  extends: parent.yaml
  base: python:3.13-slim
  runtime: gvisor
`

	if err := os.WriteFile(filepath.Join(dir, "parent.yaml"), []byte(parentContent), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child.yaml"), []byte(childContent), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	bp, err := ParseFile(filepath.Join(dir, "child.yaml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if bp.Spec.Base != "python:3.13-slim" {
		t.Errorf("expected base 'python:3.13-slim', got %q", bp.Spec.Base)
	}
	if bp.Spec.Runtime != "gvisor" {
		t.Errorf("expected runtime 'gvisor', got %q", bp.Spec.Runtime)
	}
}

func TestResolveExtendsCircularDepth(t *testing.T) {
	dir := t.TempDir()

	// Create a chain that exceeds maxExtendsDepth.
	for i := 0; i <= maxExtendsDepth+1; i++ {
		var content string
		if i == 0 {
			content = `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: bp-0
spec:
  base: alpine
  runtime: docker
`
		} else {
			content = fmt.Sprintf(`apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: bp-%d
spec:
  extends: bp-%d.yaml
`, i, i-1)
		}
		path := filepath.Join(dir, fmt.Sprintf("bp-%d.yaml", i))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write bp-%d: %v", i, err)
		}
	}

	// Parsing the deepest file should fail because it exceeds maxExtendsDepth.
	deepPath := filepath.Join(dir, fmt.Sprintf("bp-%d.yaml", maxExtendsDepth+1))
	_, err := ParseFile(deepPath)
	if err == nil {
		t.Fatal("expected error for excessive extends depth")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected 'exceeds maximum' error, got: %v", err)
	}
}

func TestResolveExtendsParentNotFound(t *testing.T) {
	dir := t.TempDir()

	childContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: child-bp
spec:
  extends: nonexistent.yaml
  base: alpine
  runtime: docker
`
	childPath := filepath.Join(dir, "child.yaml")
	if err := os.WriteFile(childPath, []byte(childContent), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	_, err := ParseFile(childPath)
	if err == nil {
		t.Fatal("expected error for missing parent")
	}
	if !strings.Contains(err.Error(), "resolve extends") {
		t.Errorf("expected 'resolve extends' error, got: %v", err)
	}
}

func TestResolveExtendsMultiLevel(t *testing.T) {
	dir := t.TempDir()

	grandparentContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: grandparent-bp
spec:
  base: python:3.12-slim
  runtime: docker
  resources:
    cpu: "1"
    memory: 256Mi
  setup:
    - run: echo grandparent
`
	parentContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: parent-bp
spec:
  extends: grandparent.yaml
  resources:
    memory: 512Mi
  setup:
    - run: echo parent
`
	childContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: child-bp
spec:
  extends: parent.yaml
  resources:
    memory: 1Gi
  setup:
    - run: echo child
`

	if err := os.WriteFile(filepath.Join(dir, "grandparent.yaml"), []byte(grandparentContent), 0o644); err != nil {
		t.Fatalf("write grandparent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent.yaml"), []byte(parentContent), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child.yaml"), []byte(childContent), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	bp, err := ParseFile(filepath.Join(dir, "child.yaml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Base and runtime inherited from grandparent through the chain.
	if bp.Spec.Base != "python:3.12-slim" {
		t.Errorf("expected base 'python:3.12-slim', got %q", bp.Spec.Base)
	}
	if bp.Spec.Runtime != "docker" {
		t.Errorf("expected runtime 'docker', got %q", bp.Spec.Runtime)
	}

	// CPU from grandparent (never overridden).
	if bp.Spec.Resources.CPU != "1" {
		t.Errorf("expected CPU '1', got %q", bp.Spec.Resources.CPU)
	}

	// Memory overridden at each level, child wins.
	if bp.Spec.Resources.Memory != "1Gi" {
		t.Errorf("expected memory '1Gi', got %q", bp.Spec.Resources.Memory)
	}

	// Setup is appended at each level: grandparent + parent + child.
	if len(bp.Spec.Setup) != 3 {
		t.Fatalf("expected 3 setup steps, got %d: %v", len(bp.Spec.Setup), bp.Spec.Setup)
	}
	if bp.Spec.Setup[0].Run != "echo grandparent" {
		t.Errorf("setup[0]: got %q", bp.Spec.Setup[0].Run)
	}
	if bp.Spec.Setup[1].Run != "echo parent" {
		t.Errorf("setup[1]: got %q", bp.Spec.Setup[1].Run)
	}
	if bp.Spec.Setup[2].Run != "echo child" {
		t.Errorf("setup[2]: got %q", bp.Spec.Setup[2].Run)
	}

	// Metadata from the child.
	if bp.Metadata.Name != "child-bp" {
		t.Errorf("expected name 'child-bp', got %q", bp.Metadata.Name)
	}
}

func TestResolveExtendsSubdirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	parentContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: parent-bp
spec:
  base: alpine
  runtime: docker
  setup:
    - run: echo parent
`
	// The child references parent via a relative path that goes up one directory.
	// filepath.Join(subdir, "../parent.yaml") resolves to dir/parent.yaml,
	// so the cleaned absolute path won't contain "..".
	childContent := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: child-bp
spec:
  extends: ../parent.yaml
`

	if err := os.WriteFile(filepath.Join(dir, "parent.yaml"), []byte(parentContent), 0o644); err != nil {
		t.Fatalf("write parent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "child.yaml"), []byte(childContent), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	bp, err := ParseFile(filepath.Join(subdir, "child.yaml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if bp.Spec.Base != "alpine" {
		t.Errorf("expected base 'alpine', got %q", bp.Spec.Base)
	}
	if bp.Spec.Setup[0].Run != "echo parent" {
		t.Errorf("expected setup from parent, got %q", bp.Spec.Setup[0].Run)
	}
}

func TestResolveExtendsNoExtends(t *testing.T) {
	dir := t.TempDir()

	content := `apiVersion: smx/v1alpha1
kind: Blueprint
metadata:
  name: standalone-bp
spec:
  base: alpine
  runtime: docker
`
	path := filepath.Join(dir, "standalone.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	bp, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if bp.Metadata.Name != "standalone-bp" {
		t.Errorf("expected name 'standalone-bp', got %q", bp.Metadata.Name)
	}
	if bp.Spec.Extends != "" {
		t.Errorf("expected no extends, got %q", bp.Spec.Extends)
	}
}

func TestMergeBlueprints(t *testing.T) {
	parent := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: "parent"},
		Spec: v1alpha1.BlueprintSpec{
			Base:    "python:3.12",
			Runtime: "docker",
			Resources: v1alpha1.Resources{
				CPU:    "2",
				Memory: "1Gi",
				Disk:   "10Gi",
				GPU:    &v1alpha1.GPUSpec{Count: 1, Driver: "nvidia"},
			},
			Setup: []v1alpha1.SetupStep{
				{Run: "parent setup"},
			},
			Toolchains: []v1alpha1.Toolchain{
				{Name: "parent-tool", Image: "tool:1"},
			},
			Workspace: v1alpha1.WorkspaceSpec{MountPath: "/work"},
			Network:   v1alpha1.NetworkSpec{Policy: "bridge"},
			Devices: []v1alpha1.DeviceMapping{
				{HostPath: "/dev/kvm"},
			},
			ReadinessProbe: &v1alpha1.ProbeConfig{Type: "tcp", Port: 80},
			Env: map[string]string{
				"FOO": "parent",
				"BAR": "parent",
			},
			Secrets: []v1alpha1.SecretRef{
				{Name: "SECRET1", Source: "env:SECRET1"},
			},
		},
	}

	child := &v1alpha1.Blueprint{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Blueprint"},
		Metadata: v1alpha1.ObjectMeta{Name: "child"},
		Spec: v1alpha1.BlueprintSpec{
			Extends: "parent.yaml",
			Resources: v1alpha1.Resources{
				Memory: "4Gi",
			},
			Setup: []v1alpha1.SetupStep{
				{Run: "child setup"},
			},
			Toolchains: []v1alpha1.Toolchain{
				{Name: "child-tool", Image: "tool:2"},
			},
			Devices: []v1alpha1.DeviceMapping{
				{HostPath: "/dev/fuse"},
			},
			Env: map[string]string{
				"FOO": "child",
				"BAZ": "child",
			},
			Secrets: []v1alpha1.SecretRef{
				{Name: "SECRET2", Source: "env:SECRET2"},
			},
		},
	}

	result := mergeBlueprints(parent, child)

	// Metadata from child.
	if result.Metadata.Name != "child" {
		t.Errorf("metadata name: got %q", result.Metadata.Name)
	}

	// Base from parent (child didn't set).
	if result.Spec.Base != "python:3.12" {
		t.Errorf("base: got %q", result.Spec.Base)
	}

	// Runtime from parent.
	if result.Spec.Runtime != "docker" {
		t.Errorf("runtime: got %q", result.Spec.Runtime)
	}

	// CPU from parent, Memory from child.
	if result.Spec.Resources.CPU != "2" {
		t.Errorf("CPU: got %q", result.Spec.Resources.CPU)
	}
	if result.Spec.Resources.Memory != "4Gi" {
		t.Errorf("Memory: got %q", result.Spec.Resources.Memory)
	}
	if result.Spec.Resources.Disk != "10Gi" {
		t.Errorf("Disk: got %q", result.Spec.Resources.Disk)
	}
	// GPU from parent (child didn't set).
	if result.Spec.Resources.GPU == nil || result.Spec.Resources.GPU.Count != 1 {
		t.Errorf("GPU: unexpected value %+v", result.Spec.Resources.GPU)
	}

	// Setup: parent + child.
	if len(result.Spec.Setup) != 2 {
		t.Fatalf("setup: got %d steps", len(result.Spec.Setup))
	}
	if result.Spec.Setup[0].Run != "parent setup" {
		t.Errorf("setup[0]: got %q", result.Spec.Setup[0].Run)
	}
	if result.Spec.Setup[1].Run != "child setup" {
		t.Errorf("setup[1]: got %q", result.Spec.Setup[1].Run)
	}

	// Toolchains: parent + child.
	if len(result.Spec.Toolchains) != 2 {
		t.Fatalf("toolchains: got %d", len(result.Spec.Toolchains))
	}

	// Workspace from parent (child didn't set).
	if result.Spec.Workspace.MountPath != "/work" {
		t.Errorf("workspace: got %q", result.Spec.Workspace.MountPath)
	}

	// Network from parent (child didn't set any field).
	if result.Spec.Network.Policy != "bridge" {
		t.Errorf("network policy: got %q", result.Spec.Network.Policy)
	}

	// Devices: parent + child.
	if len(result.Spec.Devices) != 2 {
		t.Fatalf("devices: got %d", len(result.Spec.Devices))
	}

	// ReadinessProbe from parent (child didn't set).
	if result.Spec.ReadinessProbe == nil || result.Spec.ReadinessProbe.Port != 80 {
		t.Errorf("readinessProbe: got %+v", result.Spec.ReadinessProbe)
	}

	// Env: merged, child wins on conflict.
	if result.Spec.Env["FOO"] != "child" {
		t.Errorf("env FOO: got %q", result.Spec.Env["FOO"])
	}
	if result.Spec.Env["BAR"] != "parent" {
		t.Errorf("env BAR: got %q", result.Spec.Env["BAR"])
	}
	if result.Spec.Env["BAZ"] != "child" {
		t.Errorf("env BAZ: got %q", result.Spec.Env["BAZ"])
	}

	// Secrets: parent + child.
	if len(result.Spec.Secrets) != 2 {
		t.Fatalf("secrets: got %d", len(result.Spec.Secrets))
	}

	// Extends should be cleared.
	if result.Spec.Extends != "" {
		t.Errorf("extends should be cleared, got %q", result.Spec.Extends)
	}
}

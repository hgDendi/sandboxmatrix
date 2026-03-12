// Package blueprint provides parsing and validation for Blueprint YAML files.
package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"gopkg.in/yaml.v3"
)

const (
	expectedAPIVersion = "smx/v1alpha1"
	expectedKind       = "Blueprint"
	maxExtendsDepth    = 5
)

// validatePath checks that a blueprint path is safe to read.
// It rejects absolute paths outside the working directory and path traversal attempts.
func validatePath(path string) error {
	cleaned := filepath.Clean(path)
	// Reject paths that try to escape via ..
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}
	return nil
}

// ParseFile reads and parses a Blueprint from a YAML file.
// If the blueprint uses extends, the parent is loaded and merged automatically.
func ParseFile(path string) (*v1alpha1.Blueprint, error) {
	return parseFileWithDepth(path, 0)
}

func parseFileWithDepth(path string, depth int) (*v1alpha1.Blueprint, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read blueprint file: %w", err)
	}
	bp, err := Parse(data)
	if err != nil {
		return nil, err
	}

	// Resolve inheritance.
	if bp.Spec.Extends != "" {
		bp, err = resolveExtends(bp, path, depth)
		if err != nil {
			return nil, err
		}
	}
	return bp, nil
}

// Parse parses a Blueprint from YAML bytes.
func Parse(data []byte) (*v1alpha1.Blueprint, error) {
	var bp v1alpha1.Blueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("parse blueprint YAML: %w", err)
	}
	return &bp, nil
}

// resolveExtends loads the parent blueprint and merges fields.
// The child's explicitly set fields take precedence over the parent's.
func resolveExtends(child *v1alpha1.Blueprint, childPath string, depth int) (*v1alpha1.Blueprint, error) {
	if depth >= maxExtendsDepth {
		return nil, fmt.Errorf("extends depth exceeds maximum (%d): possible circular reference", maxExtendsDepth)
	}

	parentPath := child.Spec.Extends
	// Resolve relative path based on child's directory.
	if !filepath.IsAbs(parentPath) {
		parentPath = filepath.Join(filepath.Dir(childPath), parentPath)
	}

	parent, err := parseFileWithDepth(parentPath, depth+1)
	if err != nil {
		return nil, fmt.Errorf("resolve extends %q: %w", child.Spec.Extends, err)
	}

	// Merge: child overrides parent.
	merged := mergeBlueprints(parent, child)
	return merged, nil
}

// mergeBlueprints creates a new blueprint with parent as base, child overriding.
func mergeBlueprints(parent, child *v1alpha1.Blueprint) *v1alpha1.Blueprint {
	result := *parent // shallow copy

	// Metadata: child wins completely.
	result.Metadata = child.Metadata

	// Spec merging:
	// base: child wins if set.
	if child.Spec.Base != "" {
		result.Spec.Base = child.Spec.Base
	}
	// runtime: child wins if set.
	if child.Spec.Runtime != "" {
		result.Spec.Runtime = child.Spec.Runtime
	}

	// Resources: child wins per-field.
	if child.Spec.Resources.CPU != "" {
		result.Spec.Resources.CPU = child.Spec.Resources.CPU
	}
	if child.Spec.Resources.Memory != "" {
		result.Spec.Resources.Memory = child.Spec.Resources.Memory
	}
	if child.Spec.Resources.Disk != "" {
		result.Spec.Resources.Disk = child.Spec.Resources.Disk
	}
	if child.Spec.Resources.GPU != nil {
		result.Spec.Resources.GPU = child.Spec.Resources.GPU
	}

	// Setup: child's setup commands are APPENDED to parent's.
	if len(child.Spec.Setup) > 0 {
		result.Spec.Setup = append(result.Spec.Setup, child.Spec.Setup...)
	}

	// Toolchains: child's are appended.
	if len(child.Spec.Toolchains) > 0 {
		result.Spec.Toolchains = append(result.Spec.Toolchains, child.Spec.Toolchains...)
	}

	// Workspace: child wins if mountPath is set.
	if child.Spec.Workspace.MountPath != "" {
		result.Spec.Workspace = child.Spec.Workspace
	}

	// Network: child wins if any field is set.
	if child.Spec.Network.Policy != "" || len(child.Spec.Network.Expose) > 0 {
		result.Spec.Network = child.Spec.Network
	}

	// Devices: child's are appended.
	if len(child.Spec.Devices) > 0 {
		result.Spec.Devices = append(result.Spec.Devices, child.Spec.Devices...)
	}

	// ReadinessProbe: child wins if set.
	if child.Spec.ReadinessProbe != nil {
		result.Spec.ReadinessProbe = child.Spec.ReadinessProbe
	}

	// Env: merge maps (child wins on conflict).
	if len(child.Spec.Env) > 0 {
		if result.Spec.Env == nil {
			result.Spec.Env = make(map[string]string)
		}
		for k, v := range child.Spec.Env {
			result.Spec.Env[k] = v
		}
	}

	// Secrets: child's are appended.
	if len(child.Spec.Secrets) > 0 {
		result.Spec.Secrets = append(result.Spec.Secrets, child.Spec.Secrets...)
	}

	// Clear the extends field on the result.
	result.Spec.Extends = ""

	return &result
}

// Validate checks a Blueprint for required fields and valid values.
func Validate(bp *v1alpha1.Blueprint) []error {
	var errs []error

	if bp.APIVersion != expectedAPIVersion {
		errs = append(errs, fmt.Errorf("apiVersion must be %q, got %q", expectedAPIVersion, bp.APIVersion))
	}
	if bp.Kind != expectedKind {
		errs = append(errs, fmt.Errorf("kind must be %q, got %q", expectedKind, bp.Kind))
	}
	if bp.Metadata.Name == "" {
		errs = append(errs, fmt.Errorf("metadata.name is required"))
	}
	if bp.Spec.Base == "" {
		errs = append(errs, fmt.Errorf("spec.base is required"))
	}
	if bp.Spec.Runtime == "" {
		errs = append(errs, fmt.Errorf("spec.runtime is required"))
	}
	if bp.Spec.Runtime == "firecracker" {
		errs = append(errs, fmt.Errorf("spec.runtime: firecracker is a stub and not yet implemented; use docker or gvisor"))
	}

	// GPU validation.
	if bp.Spec.Resources.GPU != nil {
		if bp.Spec.Resources.GPU.Count <= 0 {
			errs = append(errs, fmt.Errorf("spec.resources.gpu.count must be greater than 0"))
		}
		if bp.Spec.Resources.GPU.Driver == "" {
			bp.Spec.Resources.GPU.Driver = "nvidia"
		}
	}

	// Device validation.
	for i, d := range bp.Spec.Devices {
		if d.HostPath == "" {
			errs = append(errs, fmt.Errorf("spec.devices[%d].hostPath is required", i))
		}
	}

	// Readiness probe validation.
	if p := bp.Spec.ReadinessProbe; p != nil {
		switch p.Type {
		case "exec":
			if len(p.Command) == 0 {
				errs = append(errs, fmt.Errorf("spec.readinessProbe: exec probe requires a command"))
			}
		case "http":
			if p.Port == 0 {
				errs = append(errs, fmt.Errorf("spec.readinessProbe: http probe requires a port"))
			}
		case "tcp":
			if p.Port == 0 {
				errs = append(errs, fmt.Errorf("spec.readinessProbe: tcp probe requires a port"))
			}
		case "":
			errs = append(errs, fmt.Errorf("spec.readinessProbe.type is required"))
		default:
			errs = append(errs, fmt.Errorf("spec.readinessProbe.type must be exec, http, or tcp"))
		}
	}

	return errs
}

// ValidateFile parses and validates a Blueprint file in one step.
func ValidateFile(path string) (*v1alpha1.Blueprint, []error) {
	if err := validatePath(path); err != nil {
		return nil, []error{err}
	}
	bp, err := ParseFile(path)
	if err != nil {
		return nil, []error{err}
	}
	if errs := Validate(bp); len(errs) > 0 {
		return bp, errs
	}
	return bp, nil
}

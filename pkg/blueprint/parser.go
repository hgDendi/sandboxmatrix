// Package blueprint provides parsing and validation for Blueprint YAML files.
package blueprint

import (
	"fmt"
	"os"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	"gopkg.in/yaml.v3"
)

const (
	expectedAPIVersion = "smx/v1alpha1"
	expectedKind       = "Blueprint"
)

// ParseFile reads and parses a Blueprint from a YAML file.
func ParseFile(path string) (*v1alpha1.Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read blueprint file: %w", err)
	}
	return Parse(data)
}

// Parse parses a Blueprint from YAML bytes.
func Parse(data []byte) (*v1alpha1.Blueprint, error) {
	var bp v1alpha1.Blueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("parse blueprint YAML: %w", err)
	}
	return &bp, nil
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
	bp, err := ParseFile(path)
	if err != nil {
		return nil, []error{err}
	}
	if errs := Validate(bp); len(errs) > 0 {
		return bp, errs
	}
	return bp, nil
}

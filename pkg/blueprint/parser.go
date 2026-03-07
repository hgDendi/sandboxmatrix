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

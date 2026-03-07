package controller

import (
	"context"
	"fmt"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// CreateMatrix creates a new matrix with the given members. Each member's
// sandbox is created via the existing Create method, and the group is tracked
// as a coordinated unit.
func (c *Controller) CreateMatrix(ctx context.Context, name string, members []v1alpha1.MatrixMember) (*v1alpha1.Matrix, error) {
	if c.matrices == nil {
		return nil, fmt.Errorf("matrix store not configured")
	}

	// Check for duplicate matrix name.
	if _, err := c.matrices.Get(name); err == nil {
		return nil, fmt.Errorf("matrix %q already exists", name)
	}

	now := time.Now()
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      name,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: members,
		State:   v1alpha1.MatrixStateActive,
	}

	// Create each member sandbox.
	for _, member := range members {
		sandboxName := name + "-" + member.Name
		_, err := c.Create(ctx, CreateOptions{
			Name:          sandboxName,
			BlueprintPath: member.Blueprint,
		})
		if err != nil {
			// Best-effort cleanup: destroy already-created sandboxes.
			for _, prev := range members {
				prevName := name + "-" + prev.Name
				if prevName == sandboxName {
					break
				}
				_ = c.Destroy(ctx, prevName)
			}
			return nil, fmt.Errorf("create member sandbox %q: %w", sandboxName, err)
		}
	}

	if err := c.matrices.Save(mx); err != nil {
		return nil, fmt.Errorf("save matrix: %w", err)
	}

	return mx, nil
}

// StopMatrix stops all member sandboxes in a matrix.
func (c *Controller) StopMatrix(ctx context.Context, name string) error {
	if c.matrices == nil {
		return fmt.Errorf("matrix store not configured")
	}

	mx, err := c.matrices.Get(name)
	if err != nil {
		return err
	}

	if mx.State != v1alpha1.MatrixStateActive {
		return fmt.Errorf("matrix %q is not active (state: %s)", name, mx.State)
	}

	var firstErr error
	for _, member := range mx.Members {
		sandboxName := name + "-" + member.Name
		if err := c.Stop(ctx, sandboxName); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("stop member %q: %w", sandboxName, err)
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}

	mx.State = v1alpha1.MatrixStateStopped
	mx.Metadata.UpdatedAt = time.Now()
	return c.matrices.Save(mx)
}

// StartMatrix starts all member sandboxes in a stopped matrix.
func (c *Controller) StartMatrix(ctx context.Context, name string) error {
	if c.matrices == nil {
		return fmt.Errorf("matrix store not configured")
	}

	mx, err := c.matrices.Get(name)
	if err != nil {
		return err
	}

	if mx.State != v1alpha1.MatrixStateStopped {
		return fmt.Errorf("matrix %q is not stopped (state: %s)", name, mx.State)
	}

	var firstErr error
	for _, member := range mx.Members {
		sandboxName := name + "-" + member.Name
		if err := c.Start(ctx, sandboxName); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("start member %q: %w", sandboxName, err)
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}

	mx.State = v1alpha1.MatrixStateActive
	mx.Metadata.UpdatedAt = time.Now()
	return c.matrices.Save(mx)
}

// DestroyMatrix destroys all member sandboxes and removes the matrix record.
func (c *Controller) DestroyMatrix(ctx context.Context, name string) error {
	if c.matrices == nil {
		return fmt.Errorf("matrix store not configured")
	}

	mx, err := c.matrices.Get(name)
	if err != nil {
		return err
	}

	var firstErr error
	for _, member := range mx.Members {
		sandboxName := name + "-" + member.Name
		if err := c.Destroy(ctx, sandboxName); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("destroy member %q: %w", sandboxName, err)
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}

	return c.matrices.Delete(name)
}

// GetMatrix returns a matrix by name.
func (c *Controller) GetMatrix(name string) (*v1alpha1.Matrix, error) {
	if c.matrices == nil {
		return nil, fmt.Errorf("matrix store not configured")
	}
	return c.matrices.Get(name)
}

// ListMatrices returns all matrices.
func (c *Controller) ListMatrices() ([]*v1alpha1.Matrix, error) {
	if c.matrices == nil {
		return nil, fmt.Errorf("matrix store not configured")
	}
	return c.matrices.List()
}

package controller

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/hg-dendi/sandboxmatrix/internal/observability"
	"github.com/hg-dendi/sandboxmatrix/internal/quota"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// matrixNetworkName returns the conventional network name for a matrix.
func matrixNetworkName(matrixName string) string {
	return "smx-matrix-" + matrixName
}

// CreateMatrixOptions holds options for creating a matrix.
type CreateMatrixOptions struct {
	Name    string
	Members []v1alpha1.MatrixMember
	Team    string // optional: team namespace for quota enforcement
}

// CreateMatrix creates a new matrix with the given members. Each member's
// sandbox is created via the existing Create method, and the group is tracked
// as a coordinated unit. An isolated Docker network is created so that all
// member sandboxes can communicate with each other while being isolated from
// external traffic.
func (c *Controller) CreateMatrix(ctx context.Context, name string, members []v1alpha1.MatrixMember, opts ...CreateMatrixOptions) (*v1alpha1.Matrix, error) {
	ctx, span := observability.StartSpan(ctx, "controller", "CreateMatrix",
		attribute.String(observability.AttrMatrixName, name),
	)
	defer span.End()

	if c.matrices == nil {
		return nil, fmt.Errorf("matrix store not configured")
	}

	// Extract team from options if provided.
	var team string
	if len(opts) > 0 {
		team = opts[0].Team
	}

	// Check for duplicate matrix name.
	if _, err := c.matrices.Get(name); err == nil {
		return nil, fmt.Errorf("matrix %q already exists", name)
	}

	// Enforce team quota if configured.
	if team != "" && c.quotaChecker != nil {
		qr := quota.QuotaRequest{
			Matrices:  1,
			Sandboxes: len(members),
		}
		if err := c.quotaChecker.CheckQuota(ctx, team, qr); err != nil {
			return nil, fmt.Errorf("quota check: %w", err)
		}
	}

	// Create an isolated network for this matrix.
	netName := matrixNetworkName(name)
	if err := c.runtime.CreateNetwork(ctx, netName, true); err != nil {
		return nil, fmt.Errorf("create matrix network: %w", err)
	}

	now := time.Now()
	mx := &v1alpha1.Matrix{
		TypeMeta: v1alpha1.TypeMeta{APIVersion: "smx/v1alpha1", Kind: "Matrix"},
		Metadata: v1alpha1.ObjectMeta{
			Name:      name,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Spec:    v1alpha1.MatrixSpec{Team: team},
		Members: members,
		State:   v1alpha1.MatrixStateActive,
	}

	// Create each member sandbox on the isolated network.
	for _, member := range members {
		sandboxName := name + "-" + member.Name
		_, err := c.Create(ctx, CreateOptions{
			Name:          sandboxName,
			BlueprintPath: member.Blueprint,
			NetworkName:   netName,
			Team:          team,
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
			// Best-effort: remove the network.
			_ = c.runtime.DeleteNetwork(ctx, netName)
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
	ctx, span := observability.StartSpan(ctx, "controller", "StopMatrix",
		attribute.String(observability.AttrMatrixName, name),
	)
	defer span.End()

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
	ctx, span := observability.StartSpan(ctx, "controller", "StartMatrix",
		attribute.String(observability.AttrMatrixName, name),
	)
	defer span.End()

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

// DestroyMatrix destroys all member sandboxes, removes the isolated network,
// and deletes the matrix record.
func (c *Controller) DestroyMatrix(ctx context.Context, name string) error {
	ctx, span := observability.StartSpan(ctx, "controller", "DestroyMatrix",
		attribute.String(observability.AttrMatrixName, name),
	)
	defer span.End()

	if c.matrices == nil {
		return fmt.Errorf("matrix store not configured")
	}

	mx, err := c.matrices.Get(name)
	if err != nil {
		return err
	}

	// Destroy all members, continuing even if some fail, to avoid leaving
	// the matrix in an irrecoverable half-destroyed state.
	var firstErr error
	for _, member := range mx.Members {
		sandboxName := name + "-" + member.Name
		if err := c.Destroy(ctx, sandboxName); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("destroy member %q: %w", sandboxName, err)
			}
		}
	}

	// Always clean up the network and matrix record regardless of member errors.
	netName := matrixNetworkName(name)
	_ = c.runtime.DeleteNetwork(ctx, netName)

	if delErr := c.matrices.Delete(name); delErr != nil && firstErr == nil {
		firstErr = delErr
	}
	return firstErr
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

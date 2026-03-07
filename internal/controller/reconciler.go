package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// mapContainerState maps Docker container state strings to the SandboxState enum.
func mapContainerState(dockerState string) v1alpha1.SandboxState {
	switch dockerState {
	case "running":
		return v1alpha1.SandboxStateRunning
	case "exited":
		return v1alpha1.SandboxStateStopped
	case "created":
		return v1alpha1.SandboxStatePending
	default:
		return v1alpha1.SandboxStateError
	}
}

// reconcile syncs running containers from the runtime into the state store.
// For each container reported by the runtime that is not already tracked in the
// store, it creates a new Sandbox record derived from the container's labels
// and current state.
func reconcile(ctx context.Context, rt runtime.Runtime, store state.Store) error {
	containers, err := rt.List(ctx)
	if err != nil {
		return fmt.Errorf("reconcile: list runtime containers: %w", err)
	}

	for _, info := range containers {
		name := info.Labels["sandboxmatrix/sandbox"]
		if name == "" {
			// Container lacks the sandbox name label; skip it.
			continue
		}

		// Check whether the state store already tracks this sandbox.
		if _, err := store.Get(name); err == nil {
			// Already known; nothing to do.
			continue
		}

		blueprintRef := info.Labels["sandboxmatrix/blueprint"]

		now := time.Now()
		sb := &v1alpha1.Sandbox{
			TypeMeta: v1alpha1.TypeMeta{
				APIVersion: "smx/v1alpha1",
				Kind:       "Sandbox",
			},
			Metadata: v1alpha1.ObjectMeta{
				Name:      name,
				CreatedAt: now,
				UpdatedAt: now,
				Labels: map[string]string{
					"blueprint":  blueprintRef,
					"reconciled": "true",
				},
			},
			Spec: v1alpha1.SandboxSpec{
				BlueprintRef: blueprintRef,
			},
			Status: v1alpha1.SandboxStatus{
				State:     mapContainerState(info.State),
				RuntimeID: info.ID,
				IP:        info.IP,
			},
		}

		if err := store.Save(sb); err != nil {
			return fmt.Errorf("reconcile: save sandbox %q: %w", name, err)
		}
	}

	return nil
}

// Reconcile synchronises the in-memory state store with containers that are
// still present in the runtime. This should be called once during startup so
// that sandboxes created in a previous CLI session are rediscovered.
func (c *Controller) Reconcile(ctx context.Context) error {
	return reconcile(ctx, c.runtime, c.store)
}

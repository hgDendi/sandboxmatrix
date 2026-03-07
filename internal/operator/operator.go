// Package operator provides a Kubernetes operator scaffold for sandboxMatrix.
//
// In a production implementation, this package would use sigs.k8s.io/controller-runtime
// to watch and reconcile Sandbox, Matrix, and Blueprint CRDs. The current scaffold
// shows the intended architecture without pulling in heavy Kubernetes dependencies.
//
// Architecture overview:
//
//	Kubernetes API Server
//	    |
//	    v
//	controller-runtime Manager
//	    |
//	    +-- SandboxReconciler  --> watches Sandbox CRDs  --> calls internal/controller
//	    +-- MatrixReconciler   --> watches Matrix CRDs   --> orchestrates sandbox groups
//	    +-- BlueprintReconciler --> watches Blueprint CRDs --> validates and caches blueprints
//
// The operator translates declarative K8s resources into imperative calls to the
// existing sandboxMatrix controller and runtime interfaces.
package operator

import (
	"fmt"
)

// Operator is the Kubernetes operator for sandboxMatrix.
// It watches Sandbox, Matrix, and Blueprint CRDs and reconciles them.
//
// In a real implementation, this struct would embed:
//   - ctrl.Manager from sigs.k8s.io/controller-runtime
//   - A configured runtime.Runtime (Docker, Firecracker, etc.)
//   - A state.Store for persisting sandbox state
//
// The manager would be started with mgr.Start(ctx) and would run all
// registered reconcilers in parallel.
type Operator struct {
	// namespace is the Kubernetes namespace to watch. An empty string means
	// cluster-wide (requires appropriate RBAC).
	namespace string
}

// New creates a new Operator scoped to the given namespace.
//
// In a real implementation, this would:
//  1. Build a controller-runtime manager via ctrl.NewManager(cfg, ctrl.Options{})
//  2. Register the Sandbox, Matrix, and Blueprint reconcilers
//  3. Set up health/readiness probes
//  4. Configure leader election for HA deployments
func New(namespace string) *Operator {
	return &Operator{namespace: namespace}
}

// Start begins the operator's control loop.
//
// In a real implementation, this would call mgr.Start(ctx) which:
//   - Starts all registered controllers
//   - Begins watching CRD events (create/update/delete)
//   - Runs leader election if configured
//   - Blocks until the context is canceled
//
// For now, this is a placeholder that returns an error directing users
// to install the CRDs first.
func (o *Operator) Start() error {
	return fmt.Errorf("operator mode requires a running Kubernetes cluster with sandboxMatrix CRDs installed\n\n" +
		"Install CRDs first:\n" +
		"  kubectl apply -f deploy/crds/\n\n" +
		"Or install via Helm:\n" +
		"  helm install sandboxmatrix deploy/helm/sandboxmatrix/\n\n" +
		"This is a scaffold. The real operator would use sigs.k8s.io/controller-runtime.")
}

// ReconcileSandbox handles Sandbox CRD events (create, update, delete).
//
// In a real implementation, this would:
//  1. Fetch the Sandbox CR from the K8s API
//  2. If deleted, clean up the underlying container and update finalizers
//  3. If created/updated, resolve the blueprintRef to a Blueprint CR
//  4. Call internal/controller.Create or update the sandbox via the runtime
//  5. Update the Sandbox CR's .status subresource with state, runtimeID, IP
//  6. Requeue if the sandbox is in a transient state (Creating, Destroying)
//
// The reconciler would be registered with:
//
//	ctrl.NewControllerManagedBy(mgr).
//	    For(&v1alpha1.Sandbox{}).
//	    Complete(reconciler)
func (o *Operator) ReconcileSandbox(name string, spec map[string]interface{}) error {
	// Scaffold: log what we would do.
	fmt.Printf("[operator] would reconcile Sandbox %q in namespace %q\n", name, o.namespace)
	fmt.Printf("[operator] spec: %v\n", spec)

	// In a real implementation:
	// 1. Resolve spec.blueprintRef -> Blueprint CR -> BlueprintSpec
	// 2. Build runtime.CreateConfig from merged Blueprint + Sandbox spec
	// 3. Call controller.Create(ctx, opts) to create/start the container
	// 4. Update CR status: state=Running, runtimeID=<id>, ip=<ip>
	// 5. Return ctrl.Result{} or ctrl.Result{RequeueAfter: 5s} for transient states

	return nil
}

// ReconcileMatrix handles Matrix CRD events (create, update, delete).
//
// In a real implementation, this would:
//  1. Fetch the Matrix CR from the K8s API
//  2. For each member in spec.members, ensure a corresponding Sandbox CR exists
//  3. Create an isolated network if spec.networkIsolation is true
//  4. Update each member sandbox to use the matrix network
//  5. Update the Matrix CR's .status with member states
//  6. Handle member additions/removals on update
//  7. Clean up all member sandboxes and the network on delete
//
// The reconciler would own Sandbox CRs it creates, so deleting a Matrix
// cascades to its member sandboxes via ownerReferences.
func (o *Operator) ReconcileMatrix(name string, spec map[string]interface{}) error {
	// Scaffold: log what we would do.
	fmt.Printf("[operator] would reconcile Matrix %q in namespace %q\n", name, o.namespace)
	fmt.Printf("[operator] spec: %v\n", spec)

	// In a real implementation:
	// 1. List desired members from spec.members
	// 2. List existing Sandbox CRs owned by this Matrix
	// 3. Create missing Sandbox CRs with ownerReference pointing to this Matrix
	// 4. Delete Sandbox CRs that are no longer in spec.members
	// 5. If networkIsolation: create/ensure a network named "smx-matrix-<name>"
	// 6. Update Matrix status with aggregate member states

	return nil
}

// ReconcileBlueprint handles Blueprint CRD events (create, update, delete).
//
// In a real implementation, this would:
//  1. Fetch the Blueprint CR from the K8s API
//  2. Validate the blueprint spec (base image exists, resources are valid, etc.)
//  3. Optionally pre-pull the base image on nodes via a DaemonSet
//  4. Update the Blueprint CR's .status with validation results
//  5. On delete, check if any Sandbox CRs reference this blueprint and warn
//
// Blueprints are reference data -- they don't create containers themselves,
// but are referenced by Sandbox and Matrix CRs.
func (o *Operator) ReconcileBlueprint(name string, spec map[string]interface{}) error {
	// Scaffold: log what we would do.
	fmt.Printf("[operator] would reconcile Blueprint %q in namespace %q\n", name, o.namespace)
	fmt.Printf("[operator] spec: %v\n", spec)

	// In a real implementation:
	// 1. Validate spec.base is a valid container image reference
	// 2. Validate resource limits are within cluster capacity
	// 3. Update Blueprint status: validated=true/false, message=<validation errors>

	return nil
}

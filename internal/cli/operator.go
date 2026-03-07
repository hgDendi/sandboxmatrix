package cli

import (
	"fmt"

	"github.com/hg-dendi/sandboxmatrix/internal/operator"
	"github.com/spf13/cobra"
)

func newOperatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Run the sandboxMatrix Kubernetes operator",
		Long: `The operator command group manages the sandboxMatrix Kubernetes operator.

The operator watches Sandbox, Matrix, and Blueprint CRDs in a Kubernetes
cluster and reconciles them by managing container runtimes through the
standard sandboxMatrix controller.

This is a scaffold. The real operator would use sigs.k8s.io/controller-runtime.`,
	}

	cmd.AddCommand(newOperatorStartCmd())
	return cmd
}

func newOperatorStartCmd() *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the sandboxMatrix operator",
		Long: `Start the Kubernetes operator that watches and reconciles sandboxMatrix CRDs.

The operator requires:
  1. A running Kubernetes cluster
  2. CRDs installed: kubectl apply -f deploy/crds/
  3. Appropriate RBAC: kubectl apply -f deploy/rbac.yaml

For development, you can also deploy via Helm:
  helm install sandboxmatrix deploy/helm/sandboxmatrix/`,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Printf("Starting sandboxMatrix operator (namespace: %s)...\n", namespace)

			op := operator.New(namespace)
			return op.Start()
		},
	}

	cmd.Flags().StringVar(&namespace, "namespace", "sandboxmatrix", "Kubernetes namespace to watch")
	return cmd
}

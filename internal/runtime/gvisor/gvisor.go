// Package gvisor implements the Runtime interface using Docker with the gVisor (runsc) OCI runtime.
//
// On macOS, gVisor is not natively supported. This implementation delegates to Docker
// with --runtime=runsc, which requires Docker to be configured with the runsc runtime.
// The Available() method checks whether the runsc binary is installed.
package gvisor

import (
	"os/exec"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime/docker"
)

// Runtime implements runtime.Runtime by wrapping the Docker runtime
// with the gVisor (runsc) OCI runtime override.
type Runtime struct {
	*docker.Runtime
}

// New creates a new gVisor runtime backed by Docker with --runtime=runsc.
func New() (*Runtime, error) {
	d, err := docker.NewWithOptions(docker.WithOCIRuntime("runsc"))
	if err != nil {
		return nil, err
	}
	return &Runtime{Runtime: d}, nil
}

// Name returns the runtime backend name.
func (r *Runtime) Name() string { return "gvisor" }

// Available reports whether the runsc binary is installed and reachable in PATH.
func Available() bool {
	cmd := exec.Command("runsc", "--version")
	return cmd.Run() == nil
}

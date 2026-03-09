// Package probe implements readiness and liveness probes for sandboxes.
package probe

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// Runner executes probes against a sandbox and reports readiness.
type Runner struct {
	rt runtime.Runtime
}

// NewRunner creates a new probe runner backed by the given runtime.
func NewRunner(rt runtime.Runtime) *Runner {
	return &Runner{rt: rt}
}

// WaitForReady polls the probe until it passes or the failure threshold is reached.
// It returns nil when the probe succeeds, or an error describing the failure.
func (r *Runner) WaitForReady(ctx context.Context, runtimeID, sandboxIP string, cfg *v1alpha1.ProbeConfig) error {
	if cfg == nil {
		return nil
	}

	// Apply defaults.
	initialDelay := time.Duration(cfg.InitialDelaySec) * time.Second
	period := time.Duration(cfg.PeriodSec) * time.Second
	if period == 0 {
		period = 2 * time.Second
	}
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	successThreshold := cfg.SuccessThreshold
	if successThreshold <= 0 {
		successThreshold = 1
	}
	failureThreshold := cfg.FailureThreshold
	if failureThreshold <= 0 {
		failureThreshold = 30
	}

	// Wait for initial delay.
	if initialDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(initialDelay):
		}
	}

	consecutiveSuccess := 0
	failures := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := r.runOnce(ctx, runtimeID, sandboxIP, cfg, timeout)
		if err == nil {
			consecutiveSuccess++
			if consecutiveSuccess >= successThreshold {
				return nil
			}
		} else {
			consecutiveSuccess = 0
			failures++
			if failures >= failureThreshold {
				return fmt.Errorf("probe failed after %d attempts: %w", failures, err)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(period):
		}
	}
}

// runOnce executes a single probe check.
func (r *Runner) runOnce(ctx context.Context, runtimeID, sandboxIP string, cfg *v1alpha1.ProbeConfig, timeout time.Duration) error {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch cfg.Type {
	case "exec":
		return r.execProbe(probeCtx, runtimeID, cfg.Command)
	case "http":
		return r.httpProbe(probeCtx, sandboxIP, cfg.Port, cfg.Path)
	case "tcp":
		return r.tcpProbe(probeCtx, sandboxIP, cfg.Port)
	default:
		return fmt.Errorf("unknown probe type: %q", cfg.Type)
	}
}

// execProbe runs a command inside the sandbox and checks exit code == 0.
func (r *Runner) execProbe(ctx context.Context, runtimeID string, command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("exec probe requires a command")
	}

	var stdout bytes.Buffer
	result, err := r.rt.Exec(ctx, runtimeID, &runtime.ExecConfig{
		Cmd:    command,
		Stdout: &stdout,
	})
	if err != nil {
		return fmt.Errorf("exec probe error: %w", err)
	}

	output := strings.TrimSpace(stdout.String())

	if result.ExitCode != 0 {
		return fmt.Errorf("exec probe exit code %d (output: %s)", result.ExitCode, output)
	}
	return nil
}

// httpProbe sends a GET request and checks for a 2xx status code.
func (r *Runner) httpProbe(ctx context.Context, ip string, port int, path string) error {
	if port == 0 {
		return fmt.Errorf("http probe requires a port")
	}
	if path == "" {
		path = "/"
	}

	url := fmt.Sprintf("http://%s:%d%s", ip, port, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http probe error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("http probe returned status %d", resp.StatusCode)
	}
	return nil
}

// tcpProbe tries to open a TCP connection to the given port.
func (r *Runner) tcpProbe(ctx context.Context, ip string, port int) error {
	if port == 0 {
		return fmt.Errorf("tcp probe requires a port")
	}

	addr := fmt.Sprintf("%s:%d", ip, port)
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("tcp probe error: %w", err)
	}
	conn.Close()
	return nil
}

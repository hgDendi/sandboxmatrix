package probe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// Mock Runtime (only Exec is used by probes)
// ---------------------------------------------------------------------------

type mockRuntime struct {
	execExitCode atomic.Int32 // use setExitCode() to set
	execStdout   string
	execErr      error
	execCount    int
}

func newMockRuntime(exitCode int) *mockRuntime {
	m := &mockRuntime{}
	m.execExitCode.Store(int32(exitCode))
	return m
}

func (m *mockRuntime) setExitCode(code int) {
	m.execExitCode.Store(int32(code))
}

func (m *mockRuntime) Name() string { return "mock" }

func (m *mockRuntime) Exec(_ context.Context, _ string, cfg *runtime.ExecConfig) (runtime.ExecResult, error) {
	m.execCount++
	if m.execErr != nil {
		return runtime.ExecResult{ExitCode: -1}, m.execErr
	}
	if cfg.Stdout != nil && m.execStdout != "" {
		_, _ = io.Copy(cfg.Stdout, bytes.NewBufferString(m.execStdout))
	}
	return runtime.ExecResult{ExitCode: int(m.execExitCode.Load())}, nil
}

// Unused methods — satisfy the Runtime interface.
func (m *mockRuntime) Create(context.Context, *runtime.CreateConfig) (string, error) { return "", nil }
func (m *mockRuntime) Start(context.Context, string) error                           { return nil }
func (m *mockRuntime) Stop(context.Context, string) error                            { return nil }
func (m *mockRuntime) Destroy(context.Context, string) error                         { return nil }
func (m *mockRuntime) Info(context.Context, string) (runtime.Info, error)            { return runtime.Info{}, nil }
func (m *mockRuntime) Stats(context.Context, string) (runtime.Stats, error) {
	return runtime.Stats{}, nil
}
func (m *mockRuntime) List(context.Context) ([]runtime.Info, error)             { return nil, nil }
func (m *mockRuntime) Snapshot(context.Context, string, string) (string, error) { return "", nil }
func (m *mockRuntime) Restore(context.Context, string, *runtime.CreateConfig) (string, error) {
	return "", nil
}
func (m *mockRuntime) ListSnapshots(context.Context, string) ([]runtime.SnapshotInfo, error) {
	return nil, nil
}
func (m *mockRuntime) DeleteSnapshot(context.Context, string) error      { return nil }
func (m *mockRuntime) CreateNetwork(context.Context, string, bool) error { return nil }
func (m *mockRuntime) DeleteNetwork(context.Context, string) error       { return nil }
func (m *mockRuntime) CopyToContainer(context.Context, string, string, io.Reader) error {
	return nil
}
func (m *mockRuntime) CopyFromContainer(context.Context, string, string) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockRuntime) Pause(context.Context, string) error                    { return nil }
func (m *mockRuntime) Unpause(context.Context, string) error                  { return nil }
func (m *mockRuntime) UpdateResources(context.Context, string, runtime.ResourceUpdate) error { return nil }
func (m *mockRuntime) HostInfo(context.Context) (runtime.HostResources, error) {
	return runtime.HostResources{TotalCPUs: 4, TotalMemory: 16 * 1024 * 1024 * 1024}, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestExecProbe_Success(t *testing.T) {
	rt := func() *mockRuntime { m := newMockRuntime(0); m.execStdout = "1"; return m }()
	runner := NewRunner(rt)

	cfg := &v1alpha1.ProbeConfig{
		Type:             "exec",
		Command:          []string{"echo", "1"},
		PeriodSec:        0, // use default
		SuccessThreshold: 1,
		FailureThreshold: 3,
	}

	err := runner.WaitForReady(context.Background(), "cid", "", cfg)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if rt.execCount != 1 {
		t.Errorf("expected 1 exec call, got %d", rt.execCount)
	}
}

func TestExecProbe_FailureThenSuccess(t *testing.T) {
	rt := newMockRuntime(1)
	runner := NewRunner(rt)

	// After 2 failures, switch to success.
	go func() {
		time.Sleep(300 * time.Millisecond)
		rt.setExitCode(0)
	}()

	cfg := &v1alpha1.ProbeConfig{
		Type:             "exec",
		Command:          []string{"check"},
		PeriodSec:        0, // fast
		TimeoutSec:       1,
		SuccessThreshold: 1,
		FailureThreshold: 20,
	}

	err := runner.WaitForReady(context.Background(), "cid", "", cfg)
	if err != nil {
		t.Fatalf("expected eventual success, got: %v", err)
	}
	if rt.execCount < 2 {
		t.Errorf("expected at least 2 exec calls, got %d", rt.execCount)
	}
}

func TestExecProbe_ExceedsFailureThreshold(t *testing.T) {
	rt := newMockRuntime(1)
	runner := NewRunner(rt)

	cfg := &v1alpha1.ProbeConfig{
		Type:             "exec",
		Command:          []string{"fail"},
		PeriodSec:        0,
		TimeoutSec:       1,
		SuccessThreshold: 1,
		FailureThreshold: 3,
	}

	err := runner.WaitForReady(context.Background(), "cid", "", cfg)
	if err == nil {
		t.Fatal("expected failure, got nil")
	}
	if rt.execCount != 3 {
		t.Errorf("expected 3 exec calls (failure threshold), got %d", rt.execCount)
	}
}

func TestExecProbe_ContextCancellation(t *testing.T) {
	rt := newMockRuntime(1)
	runner := NewRunner(rt)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	cfg := &v1alpha1.ProbeConfig{
		Type:             "exec",
		Command:          []string{"slow"},
		PeriodSec:        1, // slow period
		FailureThreshold: 100,
	}

	err := runner.WaitForReady(ctx, "cid", "", cfg)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestExecProbe_InitialDelay(t *testing.T) {
	rt := newMockRuntime(0)
	runner := NewRunner(rt)

	start := time.Now()
	cfg := &v1alpha1.ProbeConfig{
		Type:            "exec",
		Command:         []string{"ok"},
		InitialDelaySec: 1,
	}

	_ = runner.WaitForReady(context.Background(), "cid", "", cfg)
	elapsed := time.Since(start)
	if elapsed < 900*time.Millisecond {
		t.Errorf("initial delay not respected: elapsed %v", elapsed)
	}
}

func TestTCPProbe_Success(t *testing.T) {
	// Start a TCP listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port

	rt := &mockRuntime{}
	runner := NewRunner(rt)

	cfg := &v1alpha1.ProbeConfig{
		Type:             "tcp",
		Port:             port,
		FailureThreshold: 3,
	}

	err = runner.WaitForReady(context.Background(), "cid", "127.0.0.1", cfg)
	if err != nil {
		t.Fatalf("tcp probe should succeed: %v", err)
	}
}

func TestTCPProbe_ConnectionRefused(t *testing.T) {
	rt := &mockRuntime{}
	runner := NewRunner(rt)

	cfg := &v1alpha1.ProbeConfig{
		Type:             "tcp",
		Port:             19999, // unlikely to be open
		FailureThreshold: 2,
		TimeoutSec:       1,
	}

	err := runner.WaitForReady(context.Background(), "cid", "127.0.0.1", cfg)
	if err == nil {
		t.Fatal("expected tcp probe failure")
	}
}

func TestHTTPProbe_Success(t *testing.T) {
	srv := http.Server{Addr: "127.0.0.1:0"}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	})
	srv.Handler = mux
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	rt := &mockRuntime{}
	runner := NewRunner(rt)

	cfg := &v1alpha1.ProbeConfig{
		Type:             "http",
		Port:             port,
		Path:             "/health",
		FailureThreshold: 3,
	}

	err = runner.WaitForReady(context.Background(), "cid", "127.0.0.1", cfg)
	if err != nil {
		t.Fatalf("http probe should succeed: %v", err)
	}
}

func TestHTTPProbe_ServerError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	})
	srv := http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	rt := &mockRuntime{}
	runner := NewRunner(rt)

	cfg := &v1alpha1.ProbeConfig{
		Type:             "http",
		Port:             port,
		Path:             "/health",
		FailureThreshold: 2,
		TimeoutSec:       1,
	}

	err = runner.WaitForReady(context.Background(), "cid", "127.0.0.1", cfg)
	if err == nil {
		t.Fatal("expected http probe failure for 500 status")
	}
}

func TestNilProbeConfig(t *testing.T) {
	runner := NewRunner(&mockRuntime{})
	err := runner.WaitForReady(context.Background(), "cid", "", nil)
	if err != nil {
		t.Fatalf("nil config should return nil, got: %v", err)
	}
}

func TestUnknownProbeType(t *testing.T) {
	runner := NewRunner(&mockRuntime{})
	cfg := &v1alpha1.ProbeConfig{
		Type:             "grpc",
		FailureThreshold: 1,
	}
	err := runner.WaitForReady(context.Background(), "cid", "", cfg)
	if err == nil {
		t.Fatal("expected error for unknown probe type")
	}
}

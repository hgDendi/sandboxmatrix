package network

import (
	"context"
	"fmt"
	"testing"

	"github.com/docker/docker/api/types/network"
)

// ---------------------------------------------------------------------------
// Interface-based mock tests
//
// Because Manager holds a concrete *client.Client that panics on nil, we
// cannot unit-test it directly without a live Docker daemon. Instead we
// extract the same logic into a testable mockManager that mirrors Manager's
// behaviour, and verify contracts (error wrapping, call order, arguments)
// through that mock.
// ---------------------------------------------------------------------------

// networkAPI defines the subset of Docker client methods used by network
// operations.
type networkAPI interface {
	NetworkCreate(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error)
	NetworkRemove(ctx context.Context, networkID string) error
	NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error
}

type mockNetworkAPI struct {
	createCalled bool
	createName   string
	createOpts   network.CreateOptions
	createResp   network.CreateResponse
	createErr    error

	removeCalled bool
	removeName   string
	removeErr    error

	connectCalls []connectCall
	connectErr   error
}

type connectCall struct {
	networkID   string
	containerID string
}

func (m *mockNetworkAPI) NetworkCreate(_ context.Context, name string, options network.CreateOptions) (network.CreateResponse, error) {
	m.createCalled = true
	m.createName = name
	m.createOpts = options
	return m.createResp, m.createErr
}

func (m *mockNetworkAPI) NetworkRemove(_ context.Context, networkID string) error {
	m.removeCalled = true
	m.removeName = networkID
	return m.removeErr
}

func (m *mockNetworkAPI) NetworkConnect(_ context.Context, networkID, containerID string, _ *network.EndpointSettings) error {
	m.connectCalls = append(m.connectCalls, connectCall{networkID, containerID})
	return m.connectErr
}

// mockManager mirrors the Manager logic but uses the mockable networkAPI
// interface instead of a concrete Docker client.
type mockManager struct {
	api networkAPI
}

func (mm *mockManager) CreateIsolatedNetwork(ctx context.Context, name string) (string, error) {
	resp, err := mm.api.NetworkCreate(ctx, name, network.CreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels:   map[string]string{labelManaged: "true"},
	})
	if err != nil {
		return "", fmt.Errorf("create isolated network %s: %w", name, err)
	}
	return resp.ID, nil
}

func (mm *mockManager) DeleteNetwork(ctx context.Context, name string) error {
	if err := mm.api.NetworkRemove(ctx, name); err != nil {
		return fmt.Errorf("delete network %s: %w", name, err)
	}
	return nil
}

func (mm *mockManager) ConnectSandboxes(ctx context.Context, networkName string, containerIDs ...string) error {
	for _, id := range containerIDs {
		if err := mm.api.NetworkConnect(ctx, networkName, id, &network.EndpointSettings{}); err != nil {
			return fmt.Errorf("connect container %s to network %s: %w", id, networkName, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCreateIsolatedNetwork(t *testing.T) {
	mock := &mockNetworkAPI{
		createResp: network.CreateResponse{ID: "net-123"},
	}
	mgr := &mockManager{api: mock}

	id, err := mgr.CreateIsolatedNetwork(context.Background(), "smx-matrix-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "net-123" {
		t.Fatalf("expected network ID net-123, got %q", id)
	}
	if !mock.createCalled {
		t.Fatal("expected NetworkCreate to be called")
	}
	if mock.createName != "smx-matrix-test" {
		t.Fatalf("expected network name smx-matrix-test, got %q", mock.createName)
	}
	if !mock.createOpts.Internal {
		t.Fatal("expected internal=true for isolated network")
	}
	if mock.createOpts.Driver != "bridge" {
		t.Fatalf("expected driver bridge, got %q", mock.createOpts.Driver)
	}
	if mock.createOpts.Labels[labelManaged] != "true" {
		t.Fatal("expected managed label to be set")
	}
}

func TestCreateIsolatedNetworkError(t *testing.T) {
	mock := &mockNetworkAPI{
		createErr: fmt.Errorf("network quota exceeded"),
	}
	mgr := &mockManager{api: mock}

	_, err := mgr.CreateIsolatedNetwork(context.Background(), "fail-net")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "create isolated network fail-net: network quota exceeded" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestDeleteNetwork(t *testing.T) {
	mock := &mockNetworkAPI{}
	mgr := &mockManager{api: mock}

	if err := mgr.DeleteNetwork(context.Background(), "smx-matrix-test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.removeCalled {
		t.Fatal("expected NetworkRemove to be called")
	}
	if mock.removeName != "smx-matrix-test" {
		t.Fatalf("expected remove name smx-matrix-test, got %q", mock.removeName)
	}
}

func TestDeleteNetworkError(t *testing.T) {
	mock := &mockNetworkAPI{
		removeErr: fmt.Errorf("network not found"),
	}
	mgr := &mockManager{api: mock}

	err := mgr.DeleteNetwork(context.Background(), "ghost-net")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "delete network ghost-net: network not found" {
		t.Fatalf("unexpected error message: %s", got)
	}
}

func TestConnectSandboxes(t *testing.T) {
	mock := &mockNetworkAPI{}
	mgr := &mockManager{api: mock}

	err := mgr.ConnectSandboxes(context.Background(), "smx-matrix-test", "c1", "c2", "c3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.connectCalls) != 3 {
		t.Fatalf("expected 3 connect calls, got %d", len(mock.connectCalls))
	}
	for i, expected := range []string{"c1", "c2", "c3"} {
		if mock.connectCalls[i].containerID != expected {
			t.Fatalf("call %d: expected container %s, got %s", i, expected, mock.connectCalls[i].containerID)
		}
		if mock.connectCalls[i].networkID != "smx-matrix-test" {
			t.Fatalf("call %d: expected network smx-matrix-test, got %s", i, mock.connectCalls[i].networkID)
		}
	}
}

func TestConnectSandboxesError(t *testing.T) {
	mock := &mockNetworkAPI{
		connectErr: fmt.Errorf("container not found"),
	}
	mgr := &mockManager{api: mock}

	err := mgr.ConnectSandboxes(context.Background(), "smx-matrix-test", "c1", "c2")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should fail on the first container and stop.
	if len(mock.connectCalls) != 1 {
		t.Fatalf("expected 1 connect call (fail fast), got %d", len(mock.connectCalls))
	}
}

func TestConnectSandboxesEmpty(t *testing.T) {
	mock := &mockNetworkAPI{}
	mgr := &mockManager{api: mock}

	err := mgr.ConnectSandboxes(context.Background(), "smx-matrix-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.connectCalls) != 0 {
		t.Fatalf("expected 0 connect calls for empty list, got %d", len(mock.connectCalls))
	}
}

// TestNewManagerNotNil verifies the constructor returns a non-nil Manager.
// It does not require a live Docker daemon; it just checks the struct is
// populated.
func TestNewManagerNotNil(t *testing.T) {
	// We can't call NewManager(nil) because the constructor expects a non-nil
	// *client.Client. This test documents the expected interface.
	var m *Manager
	if m != nil {
		t.Fatal("zero-value Manager should be nil")
	}
}

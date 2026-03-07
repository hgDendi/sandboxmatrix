// Package network provides Docker network management for sandbox isolation.
package network

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

const (
	labelManaged = "sandboxmatrix/managed"
)

// Manager manages Docker networks for sandbox isolation.
type Manager struct {
	client *client.Client
}

// NewManager creates a new network Manager from a Docker client.
func NewManager(cli *client.Client) *Manager {
	return &Manager{client: cli}
}

// CreateIsolatedNetwork creates a Docker bridge network with the --internal
// flag set, meaning containers on this network cannot reach external hosts.
// Returns the network ID.
func (m *Manager) CreateIsolatedNetwork(ctx context.Context, name string) (string, error) {
	resp, err := m.client.NetworkCreate(ctx, name, network.CreateOptions{
		Driver:   "bridge",
		Internal: true,
		Labels: map[string]string{
			labelManaged: "true",
		},
	})
	if err != nil {
		return "", fmt.Errorf("create isolated network %s: %w", name, err)
	}
	return resp.ID, nil
}

// DeleteNetwork removes a Docker network by name.
func (m *Manager) DeleteNetwork(ctx context.Context, name string) error {
	if err := m.client.NetworkRemove(ctx, name); err != nil {
		return fmt.Errorf("delete network %s: %w", name, err)
	}
	return nil
}

// ConnectSandboxes connects one or more containers to the named network so
// they can communicate with each other.
func (m *Manager) ConnectSandboxes(ctx context.Context, networkName string, containerIDs ...string) error {
	for _, id := range containerIDs {
		if err := m.client.NetworkConnect(ctx, networkName, id, &network.EndpointSettings{}); err != nil {
			return fmt.Errorf("connect container %s to network %s: %w", id, networkName, err)
		}
	}
	return nil
}

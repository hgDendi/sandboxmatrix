package network

import (
	"sync"
)

// ForwardedPort represents an active port forwarding rule.
type ForwardedPort struct {
	SandboxName   string `json:"sandboxName"`
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	Protocol      string `json:"protocol"`
}

// PortForwardManager manages port forwarding for sandboxes.
type PortForwardManager struct {
	mu       sync.RWMutex
	forwards map[string][]ForwardedPort // sandbox name -> ports
}

// NewPortForwardManager creates a new PortForwardManager.
func NewPortForwardManager() *PortForwardManager {
	return &PortForwardManager{
		forwards: make(map[string][]ForwardedPort),
	}
}

// ListForwards returns all forwarded ports for a sandbox.
func (m *PortForwardManager) ListForwards(sandboxName string) []ForwardedPort {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ports := m.forwards[sandboxName]
	if ports == nil {
		return []ForwardedPort{}
	}
	// Return a copy to avoid callers mutating internal state.
	result := make([]ForwardedPort, len(ports))
	copy(result, ports)
	return result
}

// ListAllForwards returns all forwarded ports across all sandboxes.
func (m *PortForwardManager) ListAllForwards() []ForwardedPort {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []ForwardedPort
	for _, ports := range m.forwards {
		result = append(result, ports...)
	}
	if result == nil {
		return []ForwardedPort{}
	}
	return result
}

// AddForward records a port forwarding.
func (m *PortForwardManager) AddForward(f ForwardedPort) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.forwards[f.SandboxName] = append(m.forwards[f.SandboxName], f)
}

// RemoveForwards removes all forwarded ports for a sandbox.
func (m *PortForwardManager) RemoveForwards(sandboxName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.forwards, sandboxName)
}

package network

import (
	"fmt"
	"sync"
)

// ServiceEntry represents a discoverable service in a matrix.
type ServiceEntry struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname"` // e.g., "backend.my-matrix.local"
	IP       string `json:"ip"`
	Port     int    `json:"port,omitempty"`
	Matrix   string `json:"matrix"`
}

// ServiceRegistry tracks services across matrix members.
type ServiceRegistry struct {
	mu       sync.RWMutex
	services map[string][]ServiceEntry // matrix name -> entries
}

// NewServiceRegistry creates a new ServiceRegistry.
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string][]ServiceEntry),
	}
}

// Register adds a service entry to the registry.
func (r *ServiceRegistry) Register(entry ServiceEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.services[entry.Matrix] = append(r.services[entry.Matrix], entry)
}

// Lookup finds a service by matrix name and service name.
func (r *ServiceRegistry) Lookup(matrix, serviceName string) (*ServiceEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := r.services[matrix]
	for i := range entries {
		if entries[i].Name == serviceName {
			// Return a copy to avoid callers mutating internal state.
			entry := entries[i]
			return &entry, nil
		}
	}
	return nil, fmt.Errorf("service %q not found in matrix %q", serviceName, matrix)
}

// ListServices returns all service entries for a matrix.
func (r *ServiceRegistry) ListServices(matrix string) []ServiceEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := r.services[matrix]
	if entries == nil {
		return []ServiceEntry{}
	}
	// Return a copy to avoid callers mutating internal state.
	result := make([]ServiceEntry, len(entries))
	copy(result, entries)
	return result
}

// DeregisterMatrix removes all service entries for a matrix.
func (r *ServiceRegistry) DeregisterMatrix(matrix string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.services, matrix)
}

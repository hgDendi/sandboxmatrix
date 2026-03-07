package runtime

import (
	"fmt"
	"sync"
)

// Registry manages available runtime backends.
type Registry struct {
	mu       sync.RWMutex
	runtimes map[string]Runtime
}

// NewRegistry creates a new runtime registry.
func NewRegistry() *Registry {
	return &Registry{
		runtimes: make(map[string]Runtime),
	}
}

// Register adds a runtime backend to the registry.
func (r *Registry) Register(rt Runtime) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := rt.Name()
	if _, exists := r.runtimes[name]; exists {
		return fmt.Errorf("runtime %q already registered", name)
	}
	r.runtimes[name] = rt
	return nil
}

// Get returns a runtime by name.
func (r *Registry) Get(name string) (Runtime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.runtimes[name]
	if !ok {
		return nil, fmt.Errorf("runtime %q not found", name)
	}
	return rt, nil
}

// List returns the names of all registered runtimes.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.runtimes))
	for name := range r.runtimes {
		names = append(names, name)
	}
	return names
}

// Package state provides sandbox state persistence.
package state

import (
	"fmt"
	"sync"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// Store defines the interface for sandbox state persistence.
type Store interface {
	Get(name string) (*v1alpha1.Sandbox, error)
	List() ([]*v1alpha1.Sandbox, error)
	Save(sb *v1alpha1.Sandbox) error
	Delete(name string) error
}

// MemoryStore is an in-memory implementation of Store.
type MemoryStore struct {
	mu        sync.RWMutex
	sandboxes map[string]*v1alpha1.Sandbox
}

// NewMemoryStore creates a new in-memory state store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sandboxes: make(map[string]*v1alpha1.Sandbox),
	}
}

func (s *MemoryStore) Get(name string) (*v1alpha1.Sandbox, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sb, ok := s.sandboxes[name]
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", name)
	}
	// Return a copy to avoid mutation.
	cp := *sb
	return &cp, nil
}

func (s *MemoryStore) List() ([]*v1alpha1.Sandbox, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*v1alpha1.Sandbox, 0, len(s.sandboxes))
	for _, sb := range s.sandboxes {
		cp := *sb
		result = append(result, &cp)
	}
	return result, nil
}

func (s *MemoryStore) Save(sb *v1alpha1.Sandbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := *sb
	s.sandboxes[sb.Metadata.Name] = &cp
	return nil
}

func (s *MemoryStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sandboxes[name]; !ok {
		return fmt.Errorf("sandbox %q not found", name)
	}
	delete(s.sandboxes, name)
	return nil
}

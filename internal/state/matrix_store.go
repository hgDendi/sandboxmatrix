package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// MatrixStore persists matrix state.
type MatrixStore interface {
	Get(name string) (*v1alpha1.Matrix, error)
	List() ([]*v1alpha1.Matrix, error)
	Save(m *v1alpha1.Matrix) error
	Delete(name string) error
}

// MemoryMatrixStore is an in-memory implementation of MatrixStore.
type MemoryMatrixStore struct {
	mu       sync.RWMutex
	matrices map[string]*v1alpha1.Matrix
}

// NewMemoryMatrixStore creates a new in-memory matrix store.
func NewMemoryMatrixStore() *MemoryMatrixStore {
	return &MemoryMatrixStore{
		matrices: make(map[string]*v1alpha1.Matrix),
	}
}

func (s *MemoryMatrixStore) Get(name string) (*v1alpha1.Matrix, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.matrices[name]
	if !ok {
		return nil, fmt.Errorf("matrix %q not found", name)
	}
	cp := *m
	cp.Members = make([]v1alpha1.MatrixMember, len(m.Members))
	copy(cp.Members, m.Members)
	return &cp, nil
}

func (s *MemoryMatrixStore) List() ([]*v1alpha1.Matrix, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*v1alpha1.Matrix, 0, len(s.matrices))
	for _, m := range s.matrices {
		cp := *m
		cp.Members = make([]v1alpha1.MatrixMember, len(m.Members))
		copy(cp.Members, m.Members)
		result = append(result, &cp)
	}
	return result, nil
}

func (s *MemoryMatrixStore) Save(m *v1alpha1.Matrix) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := *m
	cp.Members = make([]v1alpha1.MatrixMember, len(m.Members))
	copy(cp.Members, m.Members)
	s.matrices[m.Metadata.Name] = &cp
	return nil
}

func (s *MemoryMatrixStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.matrices[name]; !ok {
		return fmt.Errorf("matrix %q not found", name)
	}
	delete(s.matrices, name)
	return nil
}

// defaultMatrixPath returns ~/.sandboxmatrix/matrices.json.
func defaultMatrixPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".sandboxmatrix", "matrices.json"), nil
}

// FileMatrixStore is a JSON-file-backed implementation of MatrixStore.
// It reads state from disk on every Get/List and writes on every Save/Delete,
// using atomic writes (write-to-temp then rename) to prevent corruption.
type FileMatrixStore struct {
	mu   sync.Mutex
	path string
}

// NewFileMatrixStore creates a FileMatrixStore that persists to the default
// path (~/.sandboxmatrix/matrices.json).
func NewFileMatrixStore() (*FileMatrixStore, error) {
	p, err := defaultMatrixPath()
	if err != nil {
		return nil, err
	}
	return NewFileMatrixStoreWithPath(p)
}

// NewFileMatrixStoreWithPath creates a FileMatrixStore that persists to the
// given path. It creates the parent directory (and an empty state file) if they
// do not exist.
func NewFileMatrixStoreWithPath(path string) (*FileMatrixStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating matrices directory %s: %w", dir, err)
	}

	// If the file doesn't exist yet, seed it with an empty JSON object.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			return nil, fmt.Errorf("initializing matrices file %s: %w", path, err)
		}
	}

	return &FileMatrixStore{path: path}, nil
}

// matrixFileState is the on-disk JSON structure: a map of matrix name -> Matrix.
type matrixFileState map[string]*v1alpha1.Matrix

// load reads and unmarshals the matrices file.
func (fs *FileMatrixStore) load() (matrixFileState, error) {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		return nil, fmt.Errorf("reading matrices file: %w", err)
	}
	var state matrixFileState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing matrices file: %w", err)
	}
	if state == nil {
		state = make(matrixFileState)
	}
	return state, nil
}

// save marshals and atomically writes the matrices file.
func (fs *FileMatrixStore) save(state matrixFileState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling matrices: %w", err)
	}

	dir := filepath.Dir(fs.path)
	tmp, err := os.CreateTemp(dir, "matrices-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, fs.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming temp file to matrices file: %w", err)
	}
	return nil
}

// Get returns a copy of the named matrix, or an error if it does not exist.
func (fs *FileMatrixStore) Get(name string) (*v1alpha1.Matrix, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return nil, err
	}

	m, ok := state[name]
	if !ok {
		return nil, fmt.Errorf("matrix %q not found", name)
	}
	cp := *m
	return &cp, nil
}

// List returns copies of all matrices in the store.
func (fs *FileMatrixStore) List() ([]*v1alpha1.Matrix, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return nil, err
	}

	result := make([]*v1alpha1.Matrix, 0, len(state))
	for _, m := range state {
		cp := *m
		result = append(result, &cp)
	}
	return result, nil
}

// Save persists the given matrix, keyed by its Metadata.Name.
func (fs *FileMatrixStore) Save(m *v1alpha1.Matrix) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return err
	}

	cp := *m
	state[m.Metadata.Name] = &cp

	return fs.save(state)
}

// Delete removes the named matrix. Returns an error if it does not exist.
func (fs *FileMatrixStore) Delete(name string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return err
	}

	if _, ok := state[name]; !ok {
		return fmt.Errorf("matrix %q not found", name)
	}
	delete(state, name)

	return fs.save(state)
}

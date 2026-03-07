package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// defaultStatePath returns ~/.sandboxmatrix/state.json.
func defaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".sandboxmatrix", "state.json"), nil
}

// FileStore is a JSON-file-backed implementation of Store.
// It reads state from disk on every Get/List and writes on every Save/Delete,
// using atomic writes (write-to-temp then rename) to prevent corruption.
type FileStore struct {
	mu   sync.Mutex
	path string
}

// NewFileStore creates a FileStore that persists to the default path
// (~/.sandboxmatrix/state.json).
func NewFileStore() (*FileStore, error) {
	p, err := defaultStatePath()
	if err != nil {
		return nil, err
	}
	return NewFileStoreWithPath(p)
}

// NewFileStoreWithPath creates a FileStore that persists to the given path.
// It creates the parent directory (and an empty state file) if they do not exist.
func NewFileStoreWithPath(path string) (*FileStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating state directory %s: %w", dir, err)
	}

	// If the file doesn't exist yet, seed it with an empty JSON object.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			return nil, fmt.Errorf("initializing state file %s: %w", path, err)
		}
	}

	return &FileStore{path: path}, nil
}

// fileState is the on-disk JSON structure: a map of sandbox name -> Sandbox.
type fileState map[string]*v1alpha1.Sandbox

// load reads and unmarshals the state file.
func (fs *FileStore) load() (fileState, error) {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	var state fileState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	if state == nil {
		state = make(fileState)
	}
	return state, nil
}

// save marshals and atomically writes the state file.
// It writes to a temporary file in the same directory and then renames it,
// which is atomic on POSIX systems and avoids partial-write corruption.
func (fs *FileStore) save(state fileState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	dir := filepath.Dir(fs.path)
	tmp, err := os.CreateTemp(dir, "state-*.tmp")
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
		return fmt.Errorf("renaming temp file to state file: %w", err)
	}
	return nil
}

// Get returns a copy of the named sandbox, or an error if it does not exist.
func (fs *FileStore) Get(name string) (*v1alpha1.Sandbox, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return nil, err
	}

	sb, ok := state[name]
	if !ok {
		return nil, fmt.Errorf("sandbox %q not found", name)
	}
	cp := *sb
	return &cp, nil
}

// List returns copies of all sandboxes in the store.
func (fs *FileStore) List() ([]*v1alpha1.Sandbox, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return nil, err
	}

	result := make([]*v1alpha1.Sandbox, 0, len(state))
	for _, sb := range state {
		cp := *sb
		result = append(result, &cp)
	}
	return result, nil
}

// Save persists the given sandbox, keyed by its Metadata.Name.
func (fs *FileStore) Save(sb *v1alpha1.Sandbox) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return err
	}

	cp := *sb
	state[sb.Metadata.Name] = &cp

	return fs.save(state)
}

// Delete removes the named sandbox. Returns an error if it does not exist.
func (fs *FileStore) Delete(name string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return err
	}

	if _, ok := state[name]; !ok {
		return fmt.Errorf("sandbox %q not found", name)
	}
	delete(state, name)

	return fs.save(state)
}

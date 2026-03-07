package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// SessionStore persists session state.
type SessionStore interface {
	GetSession(id string) (*v1alpha1.Session, error)
	ListSessions() ([]*v1alpha1.Session, error)
	ListSessionsBySandbox(sandboxName string) ([]*v1alpha1.Session, error)
	SaveSession(s *v1alpha1.Session) error
	DeleteSession(id string) error
}

// defaultSessionPath returns ~/.sandboxmatrix/sessions.json.
func defaultSessionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".sandboxmatrix", "sessions.json"), nil
}

// FileSessionStore is a JSON-file-backed implementation of SessionStore.
// It reads state from disk on every Get/List and writes on every Save/Delete,
// using atomic writes (write-to-temp then rename) to prevent corruption.
type FileSessionStore struct {
	mu   sync.Mutex
	path string
}

// NewFileSessionStore creates a FileSessionStore that persists to the default
// path (~/.sandboxmatrix/sessions.json).
func NewFileSessionStore() (*FileSessionStore, error) {
	p, err := defaultSessionPath()
	if err != nil {
		return nil, err
	}
	return NewFileSessionStoreWithPath(p)
}

// NewFileSessionStoreWithPath creates a FileSessionStore that persists to the
// given path. It creates the parent directory (and an empty state file) if they
// do not exist.
func NewFileSessionStoreWithPath(path string) (*FileSessionStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating sessions directory %s: %w", dir, err)
	}

	// If the file doesn't exist yet, seed it with an empty JSON object.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			return nil, fmt.Errorf("initializing sessions file %s: %w", path, err)
		}
	}

	return &FileSessionStore{path: path}, nil
}

// sessionFileState is the on-disk JSON structure: a map of session ID -> Session.
type sessionFileState map[string]*v1alpha1.Session

// load reads and unmarshals the sessions file.
func (fs *FileSessionStore) load() (sessionFileState, error) {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		return nil, fmt.Errorf("reading sessions file: %w", err)
	}
	var state sessionFileState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing sessions file: %w", err)
	}
	if state == nil {
		state = make(sessionFileState)
	}
	return state, nil
}

// save marshals and atomically writes the sessions file.
func (fs *FileSessionStore) save(state sessionFileState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling sessions: %w", err)
	}

	dir := filepath.Dir(fs.path)
	tmp, err := os.CreateTemp(dir, "sessions-*.tmp")
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
		return fmt.Errorf("renaming temp file to sessions file: %w", err)
	}
	return nil
}

// GetSession returns a copy of the session with the given ID, or an error if
// it does not exist.
func (fs *FileSessionStore) GetSession(id string) (*v1alpha1.Session, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return nil, err
	}

	s, ok := state[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	cp := *s
	return &cp, nil
}

// ListSessions returns copies of all sessions in the store.
func (fs *FileSessionStore) ListSessions() ([]*v1alpha1.Session, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return nil, err
	}

	result := make([]*v1alpha1.Session, 0, len(state))
	for _, s := range state {
		cp := *s
		result = append(result, &cp)
	}
	return result, nil
}

// ListSessionsBySandbox returns copies of all sessions associated with the
// given sandbox name.
func (fs *FileSessionStore) ListSessionsBySandbox(sandboxName string) ([]*v1alpha1.Session, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return nil, err
	}

	var result []*v1alpha1.Session
	for _, s := range state {
		if s.Sandbox == sandboxName {
			cp := *s
			result = append(result, &cp)
		}
	}
	return result, nil
}

// SaveSession persists the given session, keyed by its Metadata.Name.
func (fs *FileSessionStore) SaveSession(s *v1alpha1.Session) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return err
	}

	cp := *s
	state[s.Metadata.Name] = &cp

	return fs.save(state)
}

// DeleteSession removes the session with the given ID. Returns an error if it
// does not exist.
func (fs *FileSessionStore) DeleteSession(id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return err
	}

	if _, ok := state[id]; !ok {
		return fmt.Errorf("session %q not found", id)
	}
	delete(state, id)

	return fs.save(state)
}

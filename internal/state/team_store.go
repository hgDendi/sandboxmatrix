package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// TeamStore persists team state.
type TeamStore interface {
	Get(name string) (*v1alpha1.Team, error)
	List() ([]*v1alpha1.Team, error)
	Save(t *v1alpha1.Team) error
	Delete(name string) error
}

// defaultTeamPath returns ~/.sandboxmatrix/teams.json.
func defaultTeamPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".sandboxmatrix", "teams.json"), nil
}

// FileTeamStore is a JSON-file-backed implementation of TeamStore.
// It reads state from disk on every Get/List and writes on every Save/Delete,
// using atomic writes (write-to-temp then rename) to prevent corruption.
type FileTeamStore struct {
	mu   sync.Mutex
	path string
}

// NewFileTeamStore creates a FileTeamStore that persists to the default
// path (~/.sandboxmatrix/teams.json).
func NewFileTeamStore() (*FileTeamStore, error) {
	p, err := defaultTeamPath()
	if err != nil {
		return nil, err
	}
	return NewFileTeamStoreWithPath(p)
}

// NewFileTeamStoreWithPath creates a FileTeamStore that persists to the
// given path. It creates the parent directory (and an empty state file) if they
// do not exist.
func NewFileTeamStoreWithPath(path string) (*FileTeamStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating teams directory %s: %w", dir, err)
	}

	// If the file doesn't exist yet, seed it with an empty JSON object.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			return nil, fmt.Errorf("initializing teams file %s: %w", path, err)
		}
	}

	return &FileTeamStore{path: path}, nil
}

// teamFileState is the on-disk JSON structure: a map of team name -> Team.
type teamFileState map[string]*v1alpha1.Team

// load reads and unmarshals the teams file.
func (fs *FileTeamStore) load() (teamFileState, error) {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		return nil, fmt.Errorf("reading teams file: %w", err)
	}
	var state teamFileState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing teams file: %w", err)
	}
	if state == nil {
		state = make(teamFileState)
	}
	return state, nil
}

// save marshals and atomically writes the teams file.
func (fs *FileTeamStore) save(state teamFileState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling teams: %w", err)
	}

	dir := filepath.Dir(fs.path)
	tmp, err := os.CreateTemp(dir, "teams-*.tmp")
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
		return fmt.Errorf("renaming temp file to teams file: %w", err)
	}
	return nil
}

// Get returns a copy of the named team, or an error if it does not exist.
func (fs *FileTeamStore) Get(name string) (*v1alpha1.Team, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return nil, err
	}

	t, ok := state[name]
	if !ok {
		return nil, fmt.Errorf("team %q not found", name)
	}
	cp := *t
	cp.Members = make([]v1alpha1.TeamMember, len(t.Members))
	copy(cp.Members, t.Members)
	return &cp, nil
}

// List returns copies of all teams in the store.
func (fs *FileTeamStore) List() ([]*v1alpha1.Team, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return nil, err
	}

	result := make([]*v1alpha1.Team, 0, len(state))
	for _, t := range state {
		cp := *t
		cp.Members = make([]v1alpha1.TeamMember, len(t.Members))
		copy(cp.Members, t.Members)
		result = append(result, &cp)
	}
	return result, nil
}

// Save persists the given team, keyed by its Name.
func (fs *FileTeamStore) Save(t *v1alpha1.Team) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return err
	}

	cp := *t
	cp.Members = make([]v1alpha1.TeamMember, len(t.Members))
	copy(cp.Members, t.Members)
	state[t.Name] = &cp

	return fs.save(state)
}

// Delete removes the named team. Returns an error if it does not exist.
func (fs *FileTeamStore) Delete(name string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	state, err := fs.load()
	if err != nil {
		return err
	}

	if _, ok := state[name]; !ok {
		return fmt.Errorf("team %q not found", name)
	}
	delete(state, name)

	return fs.save(state)
}

// MemoryTeamStore is an in-memory implementation of TeamStore.
type MemoryTeamStore struct {
	mu    sync.RWMutex
	teams map[string]*v1alpha1.Team
}

// NewMemoryTeamStore creates a new in-memory team store.
func NewMemoryTeamStore() *MemoryTeamStore {
	return &MemoryTeamStore{
		teams: make(map[string]*v1alpha1.Team),
	}
}

func (s *MemoryTeamStore) Get(name string) (*v1alpha1.Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.teams[name]
	if !ok {
		return nil, fmt.Errorf("team %q not found", name)
	}
	cp := *t
	cp.Members = make([]v1alpha1.TeamMember, len(t.Members))
	copy(cp.Members, t.Members)
	return &cp, nil
}

func (s *MemoryTeamStore) List() ([]*v1alpha1.Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*v1alpha1.Team, 0, len(s.teams))
	for _, t := range s.teams {
		cp := *t
		cp.Members = make([]v1alpha1.TeamMember, len(t.Members))
		copy(cp.Members, t.Members)
		result = append(result, &cp)
	}
	return result, nil
}

func (s *MemoryTeamStore) Save(t *v1alpha1.Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cp := *t
	cp.Members = make([]v1alpha1.TeamMember, len(t.Members))
	copy(cp.Members, t.Members)
	s.teams[t.Name] = &cp
	return nil
}

func (s *MemoryTeamStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.teams[name]; !ok {
		return fmt.Errorf("team %q not found", name)
	}
	delete(s.teams, name)
	return nil
}

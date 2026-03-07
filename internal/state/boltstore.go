package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	bbolt "go.etcd.io/bbolt"
)

var (
	bucketSandboxes = []byte("sandboxes")
	bucketSessions  = []byte("sessions")
)

// BoltStore is a BoltDB-backed implementation of both Store and SessionStore.
// It stores sandboxes and sessions as JSON-encoded values in separate buckets.
type BoltStore struct {
	db *bbolt.DB
}

// defaultBoltPath returns ~/.sandboxmatrix/state.db.
func defaultBoltPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".sandboxmatrix", "state.db"), nil
}

// NewBoltStore creates a BoltStore that persists to the default path
// (~/.sandboxmatrix/state.db).
func NewBoltStore() (*BoltStore, error) {
	p, err := defaultBoltPath()
	if err != nil {
		return nil, err
	}
	return NewBoltStoreWithPath(p)
}

// NewBoltStoreWithPath creates a BoltStore that persists to the given path.
// It creates the parent directory if it does not exist and initialises the
// required buckets.
func NewBoltStoreWithPath(path string) (*BoltStore, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating state directory %s: %w", dir, err)
	}

	db, err := bbolt.Open(path, 0o644, nil)
	if err != nil {
		return nil, fmt.Errorf("opening bolt database %s: %w", path, err)
	}

	// Ensure both buckets exist.
	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketSandboxes); err != nil {
			return fmt.Errorf("creating sandboxes bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists(bucketSessions); err != nil {
			return fmt.Errorf("creating sessions bucket: %w", err)
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, err
	}

	return &BoltStore{db: db}, nil
}

// Close closes the underlying BoltDB database.
func (s *BoltStore) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// Store interface (sandboxes)
// ---------------------------------------------------------------------------

// Get returns a copy of the named sandbox, or an error if it does not exist.
func (s *BoltStore) Get(name string) (*v1alpha1.Sandbox, error) {
	var sb v1alpha1.Sandbox
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSandboxes)
		data := b.Get([]byte(name))
		if data == nil {
			return fmt.Errorf("sandbox %q not found", name)
		}
		return json.Unmarshal(data, &sb)
	})
	if err != nil {
		return nil, err
	}
	return &sb, nil
}

// List returns copies of all sandboxes in the store.
func (s *BoltStore) List() ([]*v1alpha1.Sandbox, error) {
	var result []*v1alpha1.Sandbox
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSandboxes)
		return b.ForEach(func(k, v []byte) error {
			var sb v1alpha1.Sandbox
			if err := json.Unmarshal(v, &sb); err != nil {
				return fmt.Errorf("unmarshaling sandbox %s: %w", string(k), err)
			}
			result = append(result, &sb)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Save persists the given sandbox, keyed by its Metadata.Name.
func (s *BoltStore) Save(sb *v1alpha1.Sandbox) error {
	data, err := json.Marshal(sb)
	if err != nil {
		return fmt.Errorf("marshaling sandbox: %w", err)
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSandboxes)
		return b.Put([]byte(sb.Metadata.Name), data)
	})
}

// Delete removes the named sandbox. Returns an error if it does not exist.
func (s *BoltStore) Delete(name string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSandboxes)
		if b.Get([]byte(name)) == nil {
			return fmt.Errorf("sandbox %q not found", name)
		}
		return b.Delete([]byte(name))
	})
}

// ---------------------------------------------------------------------------
// SessionStore interface (sessions)
// ---------------------------------------------------------------------------

// GetSession returns a copy of the session with the given ID, or an error if
// it does not exist.
func (s *BoltStore) GetSession(id string) (*v1alpha1.Session, error) {
	var sess v1alpha1.Session
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		data := b.Get([]byte(id))
		if data == nil {
			return fmt.Errorf("session %q not found", id)
		}
		return json.Unmarshal(data, &sess)
	})
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// ListSessions returns copies of all sessions in the store.
func (s *BoltStore) ListSessions() ([]*v1alpha1.Session, error) {
	var result []*v1alpha1.Session
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		return b.ForEach(func(k, v []byte) error {
			var sess v1alpha1.Session
			if err := json.Unmarshal(v, &sess); err != nil {
				return fmt.Errorf("unmarshaling session %s: %w", string(k), err)
			}
			result = append(result, &sess)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ListSessionsBySandbox returns copies of all sessions associated with the
// given sandbox name.
func (s *BoltStore) ListSessionsBySandbox(sandboxName string) ([]*v1alpha1.Session, error) {
	var result []*v1alpha1.Session
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		return b.ForEach(func(k, v []byte) error {
			var sess v1alpha1.Session
			if err := json.Unmarshal(v, &sess); err != nil {
				return fmt.Errorf("unmarshaling session %s: %w", string(k), err)
			}
			if sess.Sandbox == sandboxName {
				result = append(result, &sess)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SaveSession persists the given session, keyed by its Metadata.Name.
func (s *BoltStore) SaveSession(sess *v1alpha1.Session) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		return b.Put([]byte(sess.Metadata.Name), data)
	})
}

// DeleteSession removes the session with the given ID. Returns an error if it
// does not exist.
func (s *BoltStore) DeleteSession(id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		if b.Get([]byte(id)) == nil {
			return fmt.Errorf("session %q not found", id)
		}
		return b.Delete([]byte(id))
	})
}

package state

import "fmt"

// StoreConfig configures the state store backend.
type StoreConfig struct {
	Backend       string   // "file", "bolt", "etcd"
	EtcdEndpoints []string // etcd cluster endpoints (default: ["localhost:2379"])
	FilePath      string   // path for file-based stores (empty = default)
	BoltPath      string   // path for bolt database (empty = default)
}

// NewFromConfig creates a Store based on configuration.
func NewFromConfig(cfg StoreConfig) (Store, error) {
	switch cfg.Backend {
	case "file", "":
		if cfg.FilePath != "" {
			return NewFileStoreWithPath(cfg.FilePath)
		}
		return NewFileStore()
	case "bolt":
		if cfg.BoltPath != "" {
			return NewBoltStoreWithPath(cfg.BoltPath)
		}
		return NewBoltStore()
	case "etcd":
		return NewEtcdStore(cfg.EtcdEndpoints)
	default:
		return nil, fmt.Errorf("unknown store backend %q", cfg.Backend)
	}
}

// NewSessionStoreFromConfig creates a SessionStore based on configuration.
// When using the "bolt" backend, pass the Store returned by NewFromConfig as
// existingStore to reuse the same BoltDB handle (BoltDB uses exclusive file
// locking, so opening the same file twice would deadlock).
func NewSessionStoreFromConfig(cfg StoreConfig, existingStore ...Store) (SessionStore, error) {
	switch cfg.Backend {
	case "file", "":
		if cfg.FilePath != "" {
			return NewFileSessionStoreWithPath(cfg.FilePath)
		}
		return NewFileSessionStore()
	case "bolt":
		// Reuse the existing BoltStore if provided to avoid double-open deadlock.
		for _, s := range existingStore {
			if bs, ok := s.(SessionStore); ok {
				return bs, nil
			}
		}
		if cfg.BoltPath != "" {
			return NewBoltStoreWithPath(cfg.BoltPath)
		}
		return NewBoltStore()
	case "etcd":
		return NewEtcdStore(cfg.EtcdEndpoints)
	default:
		return nil, fmt.Errorf("unknown session store backend %q", cfg.Backend)
	}
}

// NewMatrixStoreFromConfig creates a MatrixStore based on configuration.
func NewMatrixStoreFromConfig(cfg StoreConfig) (MatrixStore, error) {
	switch cfg.Backend {
	case "file", "":
		if cfg.FilePath != "" {
			return NewFileMatrixStoreWithPath(cfg.FilePath)
		}
		return NewFileMatrixStore()
	case "bolt":
		// BoltStore does not implement MatrixStore (method name conflicts).
		// Fall back to file-based matrix store alongside bolt.
		if cfg.FilePath != "" {
			return NewFileMatrixStoreWithPath(cfg.FilePath)
		}
		return NewFileMatrixStore()
	case "etcd":
		s, err := NewEtcdStore(cfg.EtcdEndpoints)
		if err != nil {
			return nil, err
		}
		return s.MatrixStore(), nil
	default:
		return nil, fmt.Errorf("unknown matrix store backend %q", cfg.Backend)
	}
}

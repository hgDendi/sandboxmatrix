package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	defaultEtcdPrefix   = "/sandboxmatrix/"
	etcdOpTimeout       = 5 * time.Second
	sandboxesKeySegment = "sandboxes/"
	sessionsKeySegment  = "sessions/"
	matricesKeySegment  = "matrices/"
)

// EtcdStore implements Store and SessionStore using etcd for distributed state.
// For MatrixStore, use the MatrixStore() method which returns an EtcdMatrixStore
// wrapper. Store and MatrixStore cannot be implemented on the same Go struct
// because they share method names (Get, List, Save, Delete) with different
// type signatures.
type EtcdStore struct {
	client *clientv3.Client
	prefix string // key prefix, default "/sandboxmatrix/"
}

// NewEtcdStore creates an EtcdStore connected to the given etcd endpoints
// using the default key prefix "/sandboxmatrix/".
func NewEtcdStore(endpoints []string) (*EtcdStore, error) {
	return NewEtcdStoreWithPrefix(endpoints, defaultEtcdPrefix)
}

// NewEtcdStoreWithPrefix creates an EtcdStore connected to the given etcd
// endpoints using the specified key prefix.
func NewEtcdStoreWithPrefix(endpoints []string, prefix string) (*EtcdStore, error) {
	if len(endpoints) == 0 {
		endpoints = []string{"localhost:2379"}
	}
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: etcdOpTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("creating etcd client: %w", err)
	}
	return &EtcdStore{
		client: cli,
		prefix: prefix,
	}, nil
}

// Close closes the underlying etcd client connection.
func (s *EtcdStore) Close() error {
	return s.client.Close()
}

// MatrixStore returns an EtcdMatrixStore backed by the same etcd client and
// prefix, implementing the MatrixStore interface.
func (s *EtcdStore) MatrixStore() *EtcdMatrixStore {
	return &EtcdMatrixStore{
		client: s.client,
		prefix: s.prefix,
	}
}

// ---------------------------------------------------------------------------
// Key helpers
// ---------------------------------------------------------------------------

func (s *EtcdStore) sandboxKey(name string) string {
	return s.prefix + sandboxesKeySegment + name
}

func (s *EtcdStore) sandboxPrefix() string {
	return s.prefix + sandboxesKeySegment
}

func (s *EtcdStore) sessionKey(id string) string {
	return s.prefix + sessionsKeySegment + id
}

func (s *EtcdStore) sessionPrefix() string {
	return s.prefix + sessionsKeySegment
}

// ---------------------------------------------------------------------------
// Store interface (sandboxes)
// ---------------------------------------------------------------------------

// Get returns the sandbox with the given name, or an error if it does not exist.
func (s *EtcdStore) Get(name string) (*v1alpha1.Sandbox, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	resp, err := s.client.Get(ctx, s.sandboxKey(name))
	if err != nil {
		return nil, fmt.Errorf("etcd get sandbox %q: %w", name, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("sandbox %q not found", name)
	}

	var sb v1alpha1.Sandbox
	if err := json.Unmarshal(resp.Kvs[0].Value, &sb); err != nil {
		return nil, fmt.Errorf("unmarshaling sandbox %q: %w", name, err)
	}
	return &sb, nil
}

// List returns all sandboxes stored under the sandbox prefix.
func (s *EtcdStore) List() ([]*v1alpha1.Sandbox, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	resp, err := s.client.Get(ctx, s.sandboxPrefix(), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd list sandboxes: %w", err)
	}

	result := make([]*v1alpha1.Sandbox, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var sb v1alpha1.Sandbox
		if err := json.Unmarshal(kv.Value, &sb); err != nil {
			return nil, fmt.Errorf("unmarshaling sandbox %s: %w", string(kv.Key), err)
		}
		result = append(result, &sb)
	}
	return result, nil
}

// Save persists the given sandbox, keyed by its Metadata.Name.
func (s *EtcdStore) Save(sb *v1alpha1.Sandbox) error {
	data, err := json.Marshal(sb)
	if err != nil {
		return fmt.Errorf("marshaling sandbox: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	_, err = s.client.Put(ctx, s.sandboxKey(sb.Metadata.Name), string(data))
	if err != nil {
		return fmt.Errorf("etcd put sandbox %q: %w", sb.Metadata.Name, err)
	}
	return nil
}

// Delete removes the named sandbox. Returns an error if it does not exist.
func (s *EtcdStore) Delete(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	resp, err := s.client.Delete(ctx, s.sandboxKey(name))
	if err != nil {
		return fmt.Errorf("etcd delete sandbox %q: %w", name, err)
	}
	if resp.Deleted == 0 {
		return fmt.Errorf("sandbox %q not found", name)
	}
	return nil
}

// ---------------------------------------------------------------------------
// SessionStore interface (sessions)
// ---------------------------------------------------------------------------

// GetSession returns the session with the given ID, or an error if it does not exist.
func (s *EtcdStore) GetSession(id string) (*v1alpha1.Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	resp, err := s.client.Get(ctx, s.sessionKey(id))
	if err != nil {
		return nil, fmt.Errorf("etcd get session %q: %w", id, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("session %q not found", id)
	}

	var sess v1alpha1.Session
	if err := json.Unmarshal(resp.Kvs[0].Value, &sess); err != nil {
		return nil, fmt.Errorf("unmarshaling session %q: %w", id, err)
	}
	return &sess, nil
}

// ListSessions returns all sessions stored under the session prefix.
func (s *EtcdStore) ListSessions() ([]*v1alpha1.Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	resp, err := s.client.Get(ctx, s.sessionPrefix(), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd list sessions: %w", err)
	}

	result := make([]*v1alpha1.Session, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var sess v1alpha1.Session
		if err := json.Unmarshal(kv.Value, &sess); err != nil {
			return nil, fmt.Errorf("unmarshaling session %s: %w", string(kv.Key), err)
		}
		result = append(result, &sess)
	}
	return result, nil
}

// ListSessionsBySandbox returns all sessions associated with the given sandbox
// name. It scans all sessions and filters client-side.
func (s *EtcdStore) ListSessionsBySandbox(sandboxName string) ([]*v1alpha1.Session, error) {
	all, err := s.ListSessions()
	if err != nil {
		return nil, err
	}

	var result []*v1alpha1.Session
	for _, sess := range all {
		if sess.Sandbox == sandboxName {
			result = append(result, sess)
		}
	}
	return result, nil
}

// SaveSession persists the given session, keyed by its Metadata.Name.
func (s *EtcdStore) SaveSession(sess *v1alpha1.Session) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	_, err = s.client.Put(ctx, s.sessionKey(sess.Metadata.Name), string(data))
	if err != nil {
		return fmt.Errorf("etcd put session %q: %w", sess.Metadata.Name, err)
	}
	return nil
}

// DeleteSession removes the session with the given ID. Returns an error if it
// does not exist.
func (s *EtcdStore) DeleteSession(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	resp, err := s.client.Delete(ctx, s.sessionKey(id))
	if err != nil {
		return fmt.Errorf("etcd delete session %q: %w", id, err)
	}
	if resp.Deleted == 0 {
		return fmt.Errorf("session %q not found", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// EtcdMatrixStore — MatrixStore backed by etcd
// ---------------------------------------------------------------------------

// EtcdMatrixStore implements MatrixStore using etcd for distributed state.
// It shares the etcd client with EtcdStore but has its own type so it can
// implement the MatrixStore interface (whose method names collide with Store).
type EtcdMatrixStore struct {
	client *clientv3.Client
	prefix string
}

func (m *EtcdMatrixStore) matrixKey(name string) string {
	return m.prefix + matricesKeySegment + name
}

func (m *EtcdMatrixStore) matrixPrefix() string {
	return m.prefix + matricesKeySegment
}

// Get returns the matrix with the given name, or an error if it does not exist.
func (m *EtcdMatrixStore) Get(name string) (*v1alpha1.Matrix, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	resp, err := m.client.Get(ctx, m.matrixKey(name))
	if err != nil {
		return nil, fmt.Errorf("etcd get matrix %q: %w", name, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("matrix %q not found", name)
	}

	var mat v1alpha1.Matrix
	if err := json.Unmarshal(resp.Kvs[0].Value, &mat); err != nil {
		return nil, fmt.Errorf("unmarshaling matrix %q: %w", name, err)
	}
	return &mat, nil
}

// List returns all matrices stored under the matrix prefix.
func (m *EtcdMatrixStore) List() ([]*v1alpha1.Matrix, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	resp, err := m.client.Get(ctx, m.matrixPrefix(), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("etcd list matrices: %w", err)
	}

	result := make([]*v1alpha1.Matrix, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var mat v1alpha1.Matrix
		if err := json.Unmarshal(kv.Value, &mat); err != nil {
			return nil, fmt.Errorf("unmarshaling matrix %s: %w", string(kv.Key), err)
		}
		result = append(result, &mat)
	}
	return result, nil
}

// Save persists the given matrix, keyed by its Metadata.Name.
func (m *EtcdMatrixStore) Save(mat *v1alpha1.Matrix) error {
	data, err := json.Marshal(mat)
	if err != nil {
		return fmt.Errorf("marshaling matrix: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	_, err = m.client.Put(ctx, m.matrixKey(mat.Metadata.Name), string(data))
	if err != nil {
		return fmt.Errorf("etcd put matrix %q: %w", mat.Metadata.Name, err)
	}
	return nil
}

// Delete removes the named matrix. Returns an error if it does not exist.
func (m *EtcdMatrixStore) Delete(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	resp, err := m.client.Delete(ctx, m.matrixKey(name))
	if err != nil {
		return fmt.Errorf("etcd delete matrix %q: %w", name, err)
	}
	if resp.Deleted == 0 {
		return fmt.Errorf("matrix %q not found", name)
	}
	return nil
}

package state

import (
	"strings"
	"testing"
)

func TestNewFromConfig_FileBackend(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFromConfig(StoreConfig{
		Backend:  "file",
		FilePath: dir + "/state.json",
	})
	if err != nil {
		t.Fatalf("NewFromConfig file: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestNewFromConfig_DefaultBackend(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFromConfig(StoreConfig{
		Backend:  "",
		FilePath: dir + "/state.json",
	})
	if err != nil {
		t.Fatalf("NewFromConfig default: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestNewFromConfig_BoltBackend(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFromConfig(StoreConfig{
		Backend:  "bolt",
		BoltPath: dir + "/state.db",
	})
	if err != nil {
		t.Fatalf("NewFromConfig bolt: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestNewFromConfig_UnknownBackend(t *testing.T) {
	_, err := NewFromConfig(StoreConfig{
		Backend: "redis",
	})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	if !strings.Contains(err.Error(), "unknown store backend") {
		t.Errorf("expected 'unknown store backend' error, got: %v", err)
	}
}

func TestNewSessionStoreFromConfig_FileBackend(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStoreFromConfig(StoreConfig{
		Backend:  "file",
		FilePath: dir + "/sessions.json",
	})
	if err != nil {
		t.Fatalf("NewSessionStoreFromConfig file: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil session store")
	}
}

func TestNewSessionStoreFromConfig_DefaultBackend(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSessionStoreFromConfig(StoreConfig{
		Backend:  "",
		FilePath: dir + "/sessions.json",
	})
	if err != nil {
		t.Fatalf("NewSessionStoreFromConfig default: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil session store")
	}
}

func TestNewSessionStoreFromConfig_BoltReusesExisting(t *testing.T) {
	dir := t.TempDir()
	// Create a bolt store first.
	boltStore, err := NewBoltStoreWithPath(dir + "/state.db")
	if err != nil {
		t.Fatalf("NewBoltStoreWithPath: %v", err)
	}

	// Pass it as existingStore. Since BoltStore implements SessionStore,
	// it should be reused.
	sessStore, err := NewSessionStoreFromConfig(StoreConfig{
		Backend:  "bolt",
		BoltPath: dir + "/state.db",
	}, boltStore)
	if err != nil {
		t.Fatalf("NewSessionStoreFromConfig bolt: %v", err)
	}
	if sessStore == nil {
		t.Fatal("expected non-nil session store")
	}
}

func TestNewSessionStoreFromConfig_UnknownBackend(t *testing.T) {
	_, err := NewSessionStoreFromConfig(StoreConfig{
		Backend: "redis",
	})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	if !strings.Contains(err.Error(), "unknown session store backend") {
		t.Errorf("expected 'unknown session store backend' error, got: %v", err)
	}
}

func TestNewMatrixStoreFromConfig_FileBackend(t *testing.T) {
	dir := t.TempDir()
	store, err := NewMatrixStoreFromConfig(StoreConfig{
		Backend:  "file",
		FilePath: dir + "/matrices.json",
	})
	if err != nil {
		t.Fatalf("NewMatrixStoreFromConfig file: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil matrix store")
	}
}

func TestNewMatrixStoreFromConfig_DefaultBackend(t *testing.T) {
	dir := t.TempDir()
	store, err := NewMatrixStoreFromConfig(StoreConfig{
		Backend:  "",
		FilePath: dir + "/matrices.json",
	})
	if err != nil {
		t.Fatalf("NewMatrixStoreFromConfig default: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil matrix store")
	}
}

func TestNewMatrixStoreFromConfig_BoltFallsBackToFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewMatrixStoreFromConfig(StoreConfig{
		Backend:  "bolt",
		FilePath: dir + "/matrices.json",
	})
	if err != nil {
		t.Fatalf("NewMatrixStoreFromConfig bolt: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil matrix store")
	}
}

func TestNewMatrixStoreFromConfig_UnknownBackend(t *testing.T) {
	_, err := NewMatrixStoreFromConfig(StoreConfig{
		Backend: "redis",
	})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	if !strings.Contains(err.Error(), "unknown matrix store backend") {
		t.Errorf("expected 'unknown matrix store backend' error, got: %v", err)
	}
}

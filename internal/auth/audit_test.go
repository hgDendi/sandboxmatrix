package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

func TestAuditLogRecord(t *testing.T) {
	audit, err := NewAuditLog(100, "")
	if err != nil {
		t.Fatalf("NewAuditLog: %v", err)
	}
	defer audit.Close()

	audit.Record(&v1alpha1.AuditEntry{
		User:     "alice",
		Action:   "sandbox.create",
		Resource: "sandbox/my-sandbox",
		Result:   "success",
	})

	entries := audit.Query("", "", 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].User != "alice" {
		t.Errorf("expected user alice, got %q", entries[0].User)
	}
	if entries[0].Action != "sandbox.create" {
		t.Errorf("expected action sandbox.create, got %q", entries[0].Action)
	}
	if entries[0].Timestamp.IsZero() {
		t.Error("expected timestamp to be set automatically")
	}
}

func TestAuditLogQueryByUser(t *testing.T) {
	audit, err := NewAuditLog(100, "")
	if err != nil {
		t.Fatalf("NewAuditLog: %v", err)
	}
	defer audit.Close()

	audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "sandbox.create", Result: "success"})
	audit.Record(&v1alpha1.AuditEntry{User: "bob", Action: "sandbox.read", Result: "success"})
	audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "matrix.create", Result: "success"})

	entries := audit.Query("alice", "", 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for alice, got %d", len(entries))
	}
	for _, e := range entries {
		if e.User != "alice" {
			t.Errorf("expected user alice, got %q", e.User)
		}
	}
}

func TestAuditLogQueryByAction(t *testing.T) {
	audit, err := NewAuditLog(100, "")
	if err != nil {
		t.Fatalf("NewAuditLog: %v", err)
	}
	defer audit.Close()

	audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "sandbox.create", Result: "success"})
	audit.Record(&v1alpha1.AuditEntry{User: "bob", Action: "sandbox.read", Result: "success"})
	audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "sandbox.create", Result: "denied"})

	entries := audit.Query("", "sandbox.create", 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for sandbox.create, got %d", len(entries))
	}
}

func TestAuditLogQueryLimit(t *testing.T) {
	audit, err := NewAuditLog(100, "")
	if err != nil {
		t.Fatalf("NewAuditLog: %v", err)
	}
	defer audit.Close()

	for i := 0; i < 10; i++ {
		audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "sandbox.read", Result: "success"})
	}

	entries := audit.Query("", "", 3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries with limit, got %d", len(entries))
	}
}

func TestAuditLogNewestFirst(t *testing.T) {
	audit, err := NewAuditLog(100, "")
	if err != nil {
		t.Fatalf("NewAuditLog: %v", err)
	}
	defer audit.Close()

	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	audit.Record(&v1alpha1.AuditEntry{Timestamp: t1, User: "alice", Action: "first", Result: "success"})
	audit.Record(&v1alpha1.AuditEntry{Timestamp: t2, User: "alice", Action: "second", Result: "success"})
	audit.Record(&v1alpha1.AuditEntry{Timestamp: t3, User: "alice", Action: "third", Result: "success"})

	entries := audit.Query("", "", 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Action != "third" {
		t.Errorf("expected newest first (third), got %q", entries[0].Action)
	}
	if entries[2].Action != "first" {
		t.Errorf("expected oldest last (first), got %q", entries[2].Action)
	}
}

func TestAuditLogMaxSize(t *testing.T) {
	audit, err := NewAuditLog(5, "")
	if err != nil {
		t.Fatalf("NewAuditLog: %v", err)
	}
	defer audit.Close()

	for i := 0; i < 10; i++ {
		audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "sandbox.read", Result: "success"})
	}

	entries := audit.Query("", "", 0)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries (maxSize), got %d", len(entries))
	}
}

func TestAuditLogFileOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	audit, err := NewAuditLog(100, path)
	if err != nil {
		t.Fatalf("NewAuditLog: %v", err)
	}

	audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "sandbox.create", Resource: "sandbox/test", Result: "success"})
	audit.Record(&v1alpha1.AuditEntry{User: "bob", Action: "sandbox.read", Resource: "sandbox/test", Result: "success"})

	if err := audit.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read the file and verify JSON lines.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines in audit file, got %d", len(lines))
	}

	var entry v1alpha1.AuditEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("unmarshal first line: %v", err)
	}
	if entry.User != "alice" {
		t.Errorf("expected user alice in file, got %q", entry.User)
	}
	if entry.Action != "sandbox.create" {
		t.Errorf("expected action sandbox.create in file, got %q", entry.Action)
	}
}

func TestAuditLogNoFile(t *testing.T) {
	// No file output should work fine.
	audit, err := NewAuditLog(100, "")
	if err != nil {
		t.Fatalf("NewAuditLog: %v", err)
	}

	audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "test", Result: "success"})

	if err := audit.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestAuditLogQueryCombinedFilters(t *testing.T) {
	audit, err := NewAuditLog(100, "")
	if err != nil {
		t.Fatalf("NewAuditLog: %v", err)
	}
	defer audit.Close()

	audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "sandbox.create", Result: "success"})
	audit.Record(&v1alpha1.AuditEntry{User: "alice", Action: "sandbox.read", Result: "success"})
	audit.Record(&v1alpha1.AuditEntry{User: "bob", Action: "sandbox.create", Result: "success"})

	// Filter by both user and action.
	entries := audit.Query("alice", "sandbox.create", 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for alice+sandbox.create, got %d", len(entries))
	}
	if entries[0].User != "alice" || entries[0].Action != "sandbox.create" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}
}

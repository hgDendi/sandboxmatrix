package auth

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// AuditLog records and stores audit entries.
type AuditLog struct {
	mu      sync.Mutex
	entries []v1alpha1.AuditEntry
	maxSize int
	file    *os.File // optional file output
}

// NewAuditLog creates a new AuditLog. maxSize limits the in-memory ring
// buffer size (0 means unlimited). filePath is optional; if non-empty,
// entries are appended as JSON lines to the given file.
func NewAuditLog(maxSize int, filePath string) (*AuditLog, error) {
	a := &AuditLog{
		maxSize: maxSize,
	}

	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		a.file = f
	}

	return a, nil
}

// Record adds an audit entry. The timestamp is set to now if zero.
func (a *AuditLog) Record(entry *v1alpha1.AuditEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	a.entries = append(a.entries, *entry)

	// Enforce ring buffer size.
	if a.maxSize > 0 && len(a.entries) > a.maxSize {
		a.entries = a.entries[len(a.entries)-a.maxSize:]
	}

	// Append to file if configured.
	if a.file != nil {
		data, err := json.Marshal(entry)
		if err == nil {
			data = append(data, '\n')
			_, _ = a.file.Write(data)
		}
	}
}

// Query returns audit entries matching the given filters. If user or action
// is empty, it matches all. limit controls the maximum number of entries
// returned (0 means all matching entries). Results are returned in reverse
// chronological order (newest first).
func (a *AuditLog) Query(user, action string, limit int) []v1alpha1.AuditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()

	var result []v1alpha1.AuditEntry

	// Iterate in reverse for newest-first ordering.
	for i := len(a.entries) - 1; i >= 0; i-- {
		entry := a.entries[i]

		if user != "" && entry.User != user {
			continue
		}
		if action != "" && entry.Action != action {
			continue
		}

		result = append(result, entry)

		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result
}

// Close closes the underlying file, if any.
func (a *AuditLog) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file != nil {
		err := a.file.Close()
		a.file = nil
		return err
	}
	return nil
}

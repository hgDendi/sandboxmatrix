package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/runtime"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

var errSessionsNotConfigured = fmt.Errorf("sessions not configured")

// generateSessionID produces a unique session ID from the sandbox name and
// the current time.
func generateSessionID(sandboxName string) string {
	return fmt.Sprintf("%s-%d", sandboxName, time.Now().UnixNano())
}

// StartSession creates a new session for a sandbox.
func (c *Controller) StartSession(ctx context.Context, sandboxName string) (*v1alpha1.Session, error) {
	if c.sessions == nil {
		return nil, errSessionsNotConfigured
	}

	// Verify the sandbox exists and is running.
	sb, err := c.store.Get(sandboxName)
	if err != nil {
		return nil, fmt.Errorf("sandbox %q not found: %w", sandboxName, err)
	}
	if sb.Status.State != v1alpha1.SandboxStateRunning {
		return nil, fmt.Errorf("sandbox %q is not running (state: %s)", sandboxName, sb.Status.State)
	}

	now := time.Now()
	session := &v1alpha1.Session{
		TypeMeta: v1alpha1.TypeMeta{
			APIVersion: "smx/v1alpha1",
			Kind:       "Session",
		},
		Metadata: v1alpha1.ObjectMeta{
			Name:      generateSessionID(sandboxName),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Sandbox:   sandboxName,
		State:     v1alpha1.SessionStateActive,
		StartedAt: &now,
		ExecCount: 0,
	}

	if err := c.sessions.SaveSession(session); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}

	return session, nil
}

// EndSession ends an active session.
func (c *Controller) EndSession(ctx context.Context, sessionID string) error {
	if c.sessions == nil {
		return errSessionsNotConfigured
	}

	session, err := c.sessions.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("session %q not found: %w", sessionID, err)
	}

	if session.State != v1alpha1.SessionStateActive {
		return fmt.Errorf("session %q is not active (state: %s)", sessionID, session.State)
	}

	now := time.Now()
	session.State = v1alpha1.SessionStateCompleted
	session.EndedAt = &now
	session.Metadata.UpdatedAt = now

	return c.sessions.SaveSession(session)
}

// GetSession returns a session by ID.
func (c *Controller) GetSession(sessionID string) (*v1alpha1.Session, error) {
	if c.sessions == nil {
		return nil, errSessionsNotConfigured
	}
	return c.sessions.GetSession(sessionID)
}

// ListSessions returns all sessions, optionally filtered by sandbox.
func (c *Controller) ListSessions(sandboxName string) ([]*v1alpha1.Session, error) {
	if c.sessions == nil {
		return nil, errSessionsNotConfigured
	}
	if sandboxName != "" {
		return c.sessions.ListSessionsBySandbox(sandboxName)
	}
	return c.sessions.ListSessions()
}

// ExecInSession executes a command and tracks it in the session.
func (c *Controller) ExecInSession(ctx context.Context, sessionID string, cfg *runtime.ExecConfig) (runtime.ExecResult, error) {
	if c.sessions == nil {
		return runtime.ExecResult{ExitCode: -1}, errSessionsNotConfigured
	}

	session, err := c.sessions.GetSession(sessionID)
	if err != nil {
		return runtime.ExecResult{ExitCode: -1}, fmt.Errorf("session %q not found: %w", sessionID, err)
	}

	if session.State != v1alpha1.SessionStateActive {
		return runtime.ExecResult{ExitCode: -1}, fmt.Errorf("session %q is not active (state: %s)", sessionID, session.State)
	}

	// Execute in the sandbox associated with this session.
	result, err := c.Exec(ctx, session.Sandbox, cfg)
	if err != nil {
		return result, err
	}

	// Update exec count.
	session.ExecCount++
	session.Metadata.UpdatedAt = time.Now()
	if saveErr := c.sessions.SaveSession(session); saveErr != nil {
		// The command succeeded but we failed to update the counter.
		// Return the result anyway but surface the save error.
		return result, fmt.Errorf("command succeeded but session update failed: %w", saveErr)
	}

	return result, nil
}

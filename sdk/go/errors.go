package sandboxmatrix

import "fmt"

// Error represents a general API error returned by the sandboxMatrix server.
type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("sandboxmatrix API error (%d): %s", e.StatusCode, e.Message)
}

// NotFoundError is returned when the requested resource does not exist.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found: %s", e.Message)
}

// NotRunningError is returned when an operation requires a running sandbox
// but the sandbox is in a different state.
type NotRunningError struct {
	Message string
}

func (e *NotRunningError) Error() string {
	return fmt.Sprintf("not running: %s", e.Message)
}

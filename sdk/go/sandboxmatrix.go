// Package sandboxmatrix provides a Go client for the sandboxMatrix REST API.
//
// The client connects to the sandboxMatrix API server to manage sandboxes,
// matrices, and sessions. Start the server with: smx server start --addr :8080
//
// Example usage:
//
//	client := sandboxmatrix.NewClient("http://localhost:8080",
//	    sandboxmatrix.WithToken("my-token"),
//	)
//
//	sb, err := client.CreateSandbox(ctx, "dev", "blueprints/python.yaml", "")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	result, err := client.Exec(ctx, "dev", "echo hello")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result.Stdout)
package sandboxmatrix

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an HTTP client for the sandboxMatrix REST API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// Option configures the Client.
type Option func(*Client)

// WithToken sets the Bearer token for RBAC authentication.
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// WithHTTPClient sets a custom http.Client for all requests.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// NewClient creates a new sandboxMatrix API client.
//
// baseURL is the address of the sandboxMatrix API server, e.g. "http://localhost:8080".
func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// request performs an HTTP request and decodes the JSON response into dst.
// If dst is nil the response body is discarded.
func (c *Client) request(ctx context.Context, method, path string, body any, dst any) error {
	reqURL := c.baseURL + "/api/v1" + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg := string(respBody)
		// Try to extract the "error" field from JSON response.
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			msg = errResp.Error
		}
		if resp.StatusCode == http.StatusNotFound {
			return &NotFoundError{Message: msg}
		}
		return &Error{StatusCode: resp.StatusCode, Message: msg}
	}

	if dst != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, dst); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// --------------------------------------------------------------------
// Health & Version
// --------------------------------------------------------------------

// Health checks the API server health.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var resp HealthResponse
	if err := c.request(ctx, http.MethodGet, "/health", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Version returns the API server version information.
func (c *Client) Version(ctx context.Context) (*VersionInfo, error) {
	var resp VersionInfo
	if err := c.request(ctx, http.MethodGet, "/version", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --------------------------------------------------------------------
// Sandbox operations
// --------------------------------------------------------------------

// CreateSandbox creates a new sandbox from a blueprint.
func (c *Client) CreateSandbox(ctx context.Context, name, blueprint, workspace string) (*Sandbox, error) {
	body := map[string]string{
		"name":      name,
		"blueprint": blueprint,
	}
	if workspace != "" {
		body["workspace"] = workspace
	}

	var raw apiSandbox
	if err := c.request(ctx, http.MethodPost, "/sandboxes", body, &raw); err != nil {
		return nil, err
	}
	sb := parseSandbox(raw)
	return &sb, nil
}

// GetSandbox retrieves a sandbox by name.
func (c *Client) GetSandbox(ctx context.Context, name string) (*Sandbox, error) {
	var raw apiSandbox
	if err := c.request(ctx, http.MethodGet, "/sandboxes/"+url.PathEscape(name), nil, &raw); err != nil {
		return nil, err
	}
	sb := parseSandbox(raw)
	return &sb, nil
}

// ListSandboxes returns all sandboxes.
func (c *Client) ListSandboxes(ctx context.Context) ([]Sandbox, error) {
	var raw []apiSandbox
	if err := c.request(ctx, http.MethodGet, "/sandboxes", nil, &raw); err != nil {
		return nil, err
	}
	result := make([]Sandbox, len(raw))
	for i, a := range raw {
		result[i] = parseSandbox(a)
	}
	return result, nil
}

// StartSandbox starts a stopped sandbox.
func (c *Client) StartSandbox(ctx context.Context, name string) error {
	return c.request(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(name)+"/start", nil, nil)
}

// StopSandbox stops a running sandbox.
func (c *Client) StopSandbox(ctx context.Context, name string) error {
	return c.request(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(name)+"/stop", nil, nil)
}

// DestroySandbox destroys a sandbox and cleans up its resources.
func (c *Client) DestroySandbox(ctx context.Context, name string) error {
	return c.request(ctx, http.MethodDelete, "/sandboxes/"+url.PathEscape(name), nil, nil)
}

// Exec executes a command in a running sandbox. The command string is wrapped
// in ["sh", "-c", command] for shell execution.
func (c *Client) Exec(ctx context.Context, name, command string) (*ExecResult, error) {
	body := map[string]any{
		"command": []string{"sh", "-c", command},
	}
	var result ExecResult
	if err := c.request(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(name)+"/exec", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ExecRaw executes a command with explicit argv in a running sandbox.
func (c *Client) ExecRaw(ctx context.Context, name string, command []string) (*ExecResult, error) {
	body := map[string]any{
		"command": command,
	}
	var result ExecResult
	if err := c.request(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(name)+"/exec", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Stats returns resource usage statistics for a running sandbox.
func (c *Client) Stats(ctx context.Context, name string) (*Stats, error) {
	var result Stats
	if err := c.request(ctx, http.MethodGet, "/sandboxes/"+url.PathEscape(name)+"/stats", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// --------------------------------------------------------------------
// Snapshot operations
// --------------------------------------------------------------------

// CreateSnapshot creates a point-in-time snapshot of a sandbox.
func (c *Client) CreateSnapshot(ctx context.Context, name, tag string) (*SnapshotResult, error) {
	body := map[string]string{}
	if tag != "" {
		body["tag"] = tag
	}
	var result SnapshotResult
	if err := c.request(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(name)+"/snapshots", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListSnapshots returns all snapshots for a sandbox.
func (c *Client) ListSnapshots(ctx context.Context, name string) ([]Snapshot, error) {
	var result []Snapshot
	if err := c.request(ctx, http.MethodGet, "/sandboxes/"+url.PathEscape(name)+"/snapshots", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// --------------------------------------------------------------------
// Matrix operations
// --------------------------------------------------------------------

// CreateMatrix creates a new matrix with the given members.
func (c *Client) CreateMatrix(ctx context.Context, name string, members []MatrixMember) (*Matrix, error) {
	body := map[string]any{
		"name":    name,
		"members": members,
	}
	var result Matrix
	if err := c.request(ctx, http.MethodPost, "/matrices", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetMatrix retrieves a matrix by name.
func (c *Client) GetMatrix(ctx context.Context, name string) (*Matrix, error) {
	var result Matrix
	if err := c.request(ctx, http.MethodGet, "/matrices/"+url.PathEscape(name), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListMatrices returns all matrices.
func (c *Client) ListMatrices(ctx context.Context) ([]Matrix, error) {
	var result []Matrix
	if err := c.request(ctx, http.MethodGet, "/matrices", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// StartMatrix starts all member sandboxes in a stopped matrix.
func (c *Client) StartMatrix(ctx context.Context, name string) error {
	return c.request(ctx, http.MethodPost, "/matrices/"+url.PathEscape(name)+"/start", nil, nil)
}

// StopMatrix stops all member sandboxes in a matrix.
func (c *Client) StopMatrix(ctx context.Context, name string) error {
	return c.request(ctx, http.MethodPost, "/matrices/"+url.PathEscape(name)+"/stop", nil, nil)
}

// DestroyMatrix destroys a matrix and all its member sandboxes.
func (c *Client) DestroyMatrix(ctx context.Context, name string) error {
	return c.request(ctx, http.MethodDelete, "/matrices/"+url.PathEscape(name), nil, nil)
}

// --------------------------------------------------------------------
// Session operations
// --------------------------------------------------------------------

// StartSession creates a new interactive session for a sandbox.
func (c *Client) StartSession(ctx context.Context, sandbox string) (*Session, error) {
	body := map[string]string{
		"sandbox": sandbox,
	}
	var result Session
	if err := c.request(ctx, http.MethodPost, "/sessions", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListSessions returns all sessions, optionally filtered by sandbox name.
func (c *Client) ListSessions(ctx context.Context, sandbox string) ([]Session, error) {
	path := "/sessions"
	if sandbox != "" {
		path += "?sandbox=" + url.QueryEscape(sandbox)
	}
	var result []Session
	if err := c.request(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// EndSession ends an active session.
func (c *Client) EndSession(ctx context.Context, id string) error {
	return c.request(ctx, http.MethodPost, "/sessions/"+url.PathEscape(id)+"/end", nil, nil)
}

// --------------------------------------------------------------------
// File operations
// --------------------------------------------------------------------

// WriteFile writes content to a file inside the sandbox at the given path.
// The content is transferred using base64 encoding to safely handle binary data.
func (c *Client) WriteFile(ctx context.Context, sandbox, path string, content io.Reader) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}

	// Encode to base64 and decode inside the sandbox for reliable binary transfer.
	encoded := base64.StdEncoding.EncodeToString(data)

	// Create parent directory if needed, then write via base64 decode.
	dir := shellQuote(pathDir(path))
	cmd := fmt.Sprintf("mkdir -p %s && echo '%s' | base64 -d > %s",
		dir, encoded, shellQuote(path))

	result, execErr := c.Exec(ctx, sandbox, cmd)
	if execErr != nil {
		return execErr
	}
	if result.ExitCode != 0 {
		return &Error{
			StatusCode: 0,
			Message:    fmt.Sprintf("write file failed (exit %d): %s", result.ExitCode, result.Stderr),
		}
	}
	return nil
}

// ReadFile reads a file from the sandbox at the given path.
func (c *Client) ReadFile(ctx context.Context, sandbox, path string) ([]byte, error) {
	// Read file as base64 to handle binary content.
	cmd := fmt.Sprintf("base64 %s", shellQuote(path))
	result, err := c.Exec(ctx, sandbox, cmd)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, &Error{
			StatusCode: 0,
			Message:    fmt.Sprintf("read file failed (exit %d): %s", result.ExitCode, result.Stderr),
		}
	}

	// Remove any whitespace from base64 output (line-wrapped by some implementations).
	cleaned := strings.Join(strings.Fields(result.Stdout), "")
	return base64.StdEncoding.DecodeString(cleaned)
}

// ListFiles lists files in a directory inside the sandbox.
func (c *Client) ListFiles(ctx context.Context, sandbox, path string) ([]FileInfo, error) {
	// Use a Python one-liner for reliable structured output. Falls back to
	// an empty list if Python is not available.
	escaped := strings.ReplaceAll(path, "'", "'\\''")
	cmd := fmt.Sprintf(`python3 -c "
import os, json, sys
entries = []
d = '%s'
for name in sorted(os.listdir(d)):
    full = os.path.join(d, name)
    try:
        st = os.stat(full)
        entries.append({'name': name, 'path': full, 'size': st.st_size, 'isDir': os.path.isdir(full)})
    except: pass
print(json.dumps(entries))
" 2>/dev/null`, escaped)

	result, err := c.Exec(ctx, sandbox, cmd)
	if err != nil {
		return nil, err
	}

	stdout := strings.TrimSpace(result.Stdout)
	if result.ExitCode != 0 || stdout == "" {
		// Fallback: use ls + awk for basic listing.
		cmd = fmt.Sprintf(`ls -1p %s 2>/dev/null`, shellQuote(path))
		result, err = c.Exec(ctx, sandbox, cmd)
		if err != nil {
			return nil, err
		}
		if result.ExitCode != 0 {
			return nil, &Error{
				StatusCode: 0,
				Message:    fmt.Sprintf("list files failed (exit %d): %s", result.ExitCode, result.Stderr),
			}
		}
		return parseLsOutput(path, result.Stdout), nil
	}

	var files []FileInfo
	if err := json.Unmarshal([]byte(stdout), &files); err != nil {
		return nil, fmt.Errorf("parse file listing: %w", err)
	}
	return files, nil
}

// shellQuote wraps a string in single quotes for safe shell usage.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// pathDir returns the directory portion of a path.
func pathDir(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return "/"
	}
	return p[:idx]
}

// parseLsOutput converts basic ls -1p output into FileInfo entries.
func parseLsOutput(dir, output string) []FileInfo {
	var files []FileInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		isDir := strings.HasSuffix(line, "/")
		name := strings.TrimSuffix(line, "/")
		fullPath := dir
		if !strings.HasSuffix(fullPath, "/") {
			fullPath += "/"
		}
		fullPath += name
		files = append(files, FileInfo{
			Name:  name,
			Path:  fullPath,
			IsDir: isDir,
		})
	}
	return files
}

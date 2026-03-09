// Package auth provides role-based access control and audit logging.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sync"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// Permission defines what actions a role can perform.
type Permission struct {
	Resource string   // "sandbox", "matrix", "session", "pool", "*"
	Actions  []string // "create", "read", "update", "delete", "exec", "*"
}

// RBAC manages role-based access control.
type RBAC struct {
	mu    sync.RWMutex
	users map[string]*v1alpha1.User
	roles map[v1alpha1.Role][]Permission
}

// New creates a new RBAC manager with default role permissions.
func New() *RBAC {
	r := &RBAC{
		users: make(map[string]*v1alpha1.User),
		roles: map[v1alpha1.Role][]Permission{
			v1alpha1.RoleAdmin: {
				{Resource: "*", Actions: []string{"*"}},
			},
			v1alpha1.RoleOperator: {
				{Resource: "sandbox", Actions: []string{"create", "read", "update", "delete", "exec"}},
				{Resource: "matrix", Actions: []string{"create", "read", "update", "delete"}},
				{Resource: "session", Actions: []string{"create", "read", "update", "delete", "exec"}},
				{Resource: "pool", Actions: []string{"create", "read", "update", "delete"}},
				{Resource: "blueprint", Actions: []string{"read"}},
			},
			v1alpha1.RoleViewer: {
				{Resource: "*", Actions: []string{"read"}},
			},
		},
	}
	return r
}

// GenerateToken creates a cryptographically random token.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// AddUser adds a user to the RBAC system. If the user's Token is empty,
// a new token is generated automatically.
func (r *RBAC) AddUser(user *v1alpha1.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if user.Name == "" {
		return fmt.Errorf("user name is required")
	}
	if _, exists := r.users[user.Name]; exists {
		return fmt.Errorf("user %q already exists", user.Name)
	}
	if _, ok := r.roles[user.Role]; !ok {
		return fmt.Errorf("unknown role %q", user.Role)
	}

	if user.Token == "" {
		token, err := GenerateToken()
		if err != nil {
			return err
		}
		user.Token = token
	}

	cp := *user
	r.users[user.Name] = &cp
	return nil
}

// RemoveUser removes a user from the RBAC system.
func (r *RBAC) RemoveUser(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[name]; !exists {
		return fmt.Errorf("user %q not found", name)
	}
	delete(r.users, name)
	return nil
}

// GetUser returns a copy of a user by name.
func (r *RBAC) GetUser(name string) (*v1alpha1.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.users[name]
	if !ok {
		return nil, fmt.Errorf("user %q not found", name)
	}
	cp := *u
	return &cp, nil
}

// ListUsers returns copies of all users (tokens are redacted).
func (r *RBAC) ListUsers() []*v1alpha1.User {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*v1alpha1.User, 0, len(r.users))
	for _, u := range r.users {
		cp := *u
		// Redact token for listing.
		if len(cp.Token) > 8 {
			cp.Token = cp.Token[:8] + "..."
		}
		result = append(result, &cp)
	}
	return result
}

// Authorize checks if a user has permission to perform an action on a resource.
func (r *RBAC) Authorize(userName, resource, action string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.users[userName]
	if !ok {
		return fmt.Errorf("user %q not found", userName)
	}

	return r.checkPermission(user.Role, resource, action)
}

// AuthorizeByToken looks up a user by their token and checks permissions.
// Returns the user name on success.
func (r *RBAC) AuthorizeByToken(token, resource, action string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if token == "" {
		return "", fmt.Errorf("token is required")
	}

	var user *v1alpha1.User
	for _, u := range r.users {
		if subtle.ConstantTimeCompare([]byte(u.Token), []byte(token)) == 1 {
			user = u
			break
		}
	}
	if user == nil {
		return "", fmt.Errorf("invalid token")
	}

	if err := r.checkPermission(user.Role, resource, action); err != nil {
		return user.Name, err
	}
	return user.Name, nil
}

// checkPermission verifies a role has permission for the given resource/action.
// Must be called with r.mu held (at least RLock).
func (r *RBAC) checkPermission(role v1alpha1.Role, resource, action string) error {
	perms, ok := r.roles[role]
	if !ok {
		return fmt.Errorf("unknown role %q", role)
	}

	for _, perm := range perms {
		if perm.Resource == "*" || perm.Resource == resource {
			for _, a := range perm.Actions {
				if a == "*" || a == action {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("role %q does not have permission %s/%s", role, resource, action)
}

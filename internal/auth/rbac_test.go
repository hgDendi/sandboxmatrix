package auth

import (
	"testing"

	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

func TestAdminCanDoEverything(t *testing.T) {
	rbac := New()

	admin := &v1alpha1.User{Name: "admin-user", Role: v1alpha1.RoleAdmin, Token: "admin-token-123"}
	if err := rbac.AddUser(admin); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	tests := []struct {
		resource string
		action   string
	}{
		{"sandbox", "create"},
		{"sandbox", "read"},
		{"sandbox", "delete"},
		{"sandbox", "exec"},
		{"matrix", "create"},
		{"matrix", "destroy"},
		{"session", "create"},
		{"session", "exec"},
		{"pool", "create"},
		{"pool", "delete"},
		{"blueprint", "read"},
		{"user", "create"},
		{"user", "delete"},
		{"anything", "whatever"},
	}

	for _, tt := range tests {
		if err := rbac.Authorize("admin-user", tt.resource, tt.action); err != nil {
			t.Errorf("admin should have access to %s/%s, got: %v", tt.resource, tt.action, err)
		}
	}
}

func TestOperatorCanManageSandboxesButNotUsers(t *testing.T) {
	rbac := New()

	op := &v1alpha1.User{Name: "op-user", Role: v1alpha1.RoleOperator, Token: "op-token-123"}
	if err := rbac.AddUser(op); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	// Operator should be able to manage sandboxes.
	allowed := []struct {
		resource string
		action   string
	}{
		{"sandbox", "create"},
		{"sandbox", "read"},
		{"sandbox", "update"},
		{"sandbox", "delete"},
		{"sandbox", "exec"},
		{"matrix", "create"},
		{"matrix", "read"},
		{"matrix", "delete"},
		{"session", "create"},
		{"session", "exec"},
		{"pool", "create"},
		{"pool", "delete"},
		{"blueprint", "read"},
	}

	for _, tt := range allowed {
		if err := rbac.Authorize("op-user", tt.resource, tt.action); err != nil {
			t.Errorf("operator should have access to %s/%s, got: %v", tt.resource, tt.action, err)
		}
	}

	// Operator should NOT be able to manage users or do admin things.
	denied := []struct {
		resource string
		action   string
	}{
		{"user", "create"},
		{"user", "delete"},
		{"blueprint", "create"},
		{"blueprint", "delete"},
	}

	for _, tt := range denied {
		if err := rbac.Authorize("op-user", tt.resource, tt.action); err == nil {
			t.Errorf("operator should NOT have access to %s/%s", tt.resource, tt.action)
		}
	}
}

func TestViewerCanOnlyRead(t *testing.T) {
	rbac := New()

	viewer := &v1alpha1.User{Name: "viewer-user", Role: v1alpha1.RoleViewer, Token: "viewer-token-123"}
	if err := rbac.AddUser(viewer); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	// Viewer should be able to read anything.
	readAllowed := []string{"sandbox", "matrix", "session", "pool", "blueprint", "user"}
	for _, resource := range readAllowed {
		if err := rbac.Authorize("viewer-user", resource, "read"); err != nil {
			t.Errorf("viewer should have read access to %s, got: %v", resource, err)
		}
	}

	// Viewer should NOT be able to create/delete/exec.
	writeDenied := []struct {
		resource string
		action   string
	}{
		{"sandbox", "create"},
		{"sandbox", "delete"},
		{"sandbox", "exec"},
		{"matrix", "create"},
		{"matrix", "delete"},
		{"session", "create"},
		{"pool", "create"},
		{"user", "create"},
	}

	for _, tt := range writeDenied {
		if err := rbac.Authorize("viewer-user", tt.resource, tt.action); err == nil {
			t.Errorf("viewer should NOT have access to %s/%s", tt.resource, tt.action)
		}
	}
}

func TestUnknownUserIsDenied(t *testing.T) {
	rbac := New()

	if err := rbac.Authorize("nobody", "sandbox", "read"); err == nil {
		t.Error("expected error for unknown user, got nil")
	}
}

func TestTokenBasedAuth(t *testing.T) {
	rbac := New()

	admin := &v1alpha1.User{Name: "token-admin", Role: v1alpha1.RoleAdmin, Token: "secret-admin-token"}
	if err := rbac.AddUser(admin); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	viewer := &v1alpha1.User{Name: "token-viewer", Role: v1alpha1.RoleViewer, Token: "secret-viewer-token"}
	if err := rbac.AddUser(viewer); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	// Valid admin token should be authorized for everything.
	name, err := rbac.AuthorizeByToken("secret-admin-token", "sandbox", "create")
	if err != nil {
		t.Errorf("admin token should be authorized: %v", err)
	}
	if name != "token-admin" {
		t.Errorf("expected user name 'token-admin', got %q", name)
	}

	// Valid viewer token should be authorized for read.
	name, err = rbac.AuthorizeByToken("secret-viewer-token", "sandbox", "read")
	if err != nil {
		t.Errorf("viewer token should be authorized for read: %v", err)
	}
	if name != "token-viewer" {
		t.Errorf("expected user name 'token-viewer', got %q", name)
	}

	// Viewer token should be denied for create.
	name, err = rbac.AuthorizeByToken("secret-viewer-token", "sandbox", "create")
	if err == nil {
		t.Error("viewer token should NOT be authorized for create")
	}
	if name != "token-viewer" {
		t.Errorf("expected user name 'token-viewer' even on deny, got %q", name)
	}

	// Invalid token should fail.
	_, err = rbac.AuthorizeByToken("invalid-token", "sandbox", "read")
	if err == nil {
		t.Error("expected error for invalid token")
	}

	// Empty token should fail.
	_, err = rbac.AuthorizeByToken("", "sandbox", "read")
	if err == nil {
		t.Error("expected error for empty token")
	}
}

func TestAddUserDuplicate(t *testing.T) {
	rbac := New()

	user := &v1alpha1.User{Name: "dup-user", Role: v1alpha1.RoleViewer, Token: "token1"}
	if err := rbac.AddUser(user); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	user2 := &v1alpha1.User{Name: "dup-user", Role: v1alpha1.RoleAdmin, Token: "token2"}
	if err := rbac.AddUser(user2); err == nil {
		t.Error("expected error for duplicate user")
	}
}

func TestAddUserInvalidRole(t *testing.T) {
	rbac := New()

	user := &v1alpha1.User{Name: "bad-role", Role: "superuser", Token: "token"}
	if err := rbac.AddUser(user); err == nil {
		t.Error("expected error for invalid role")
	}
}

func TestAddUserGeneratesToken(t *testing.T) {
	rbac := New()

	user := &v1alpha1.User{Name: "auto-token", Role: v1alpha1.RoleViewer}
	if err := rbac.AddUser(user); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	got, err := rbac.GetUser("auto-token")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.Token == "" {
		t.Error("expected token to be auto-generated")
	}
	if len(got.Token) < 32 {
		t.Errorf("expected token length >= 32, got %d", len(got.Token))
	}
}

func TestRemoveUser(t *testing.T) {
	rbac := New()

	user := &v1alpha1.User{Name: "remove-me", Role: v1alpha1.RoleViewer, Token: "token"}
	if err := rbac.AddUser(user); err != nil {
		t.Fatalf("AddUser: %v", err)
	}

	if err := rbac.RemoveUser("remove-me"); err != nil {
		t.Fatalf("RemoveUser: %v", err)
	}

	if _, err := rbac.GetUser("remove-me"); err == nil {
		t.Error("expected error after remove")
	}

	// Remove non-existent user.
	if err := rbac.RemoveUser("nobody"); err == nil {
		t.Error("expected error for removing non-existent user")
	}
}

func TestListUsers(t *testing.T) {
	rbac := New()

	users := []*v1alpha1.User{
		{Name: "alice", Role: v1alpha1.RoleAdmin, Token: "long-token-abcdefgh12345678"},
		{Name: "bob", Role: v1alpha1.RoleViewer, Token: "short"},
	}
	for _, u := range users {
		if err := rbac.AddUser(u); err != nil {
			t.Fatalf("AddUser %s: %v", u.Name, err)
		}
	}

	list := rbac.ListUsers()
	if len(list) != 2 {
		t.Fatalf("expected 2 users, got %d", len(list))
	}

	// Tokens should be redacted (truncated for long tokens).
	for _, u := range list {
		if u.Name == "alice" && u.Token == "long-token-abcdefgh12345678" {
			t.Error("expected alice's token to be redacted")
		}
	}
}

func TestAddUserEmptyName(t *testing.T) {
	rbac := New()

	user := &v1alpha1.User{Name: "", Role: v1alpha1.RoleViewer, Token: "token"}
	if err := rbac.AddUser(user); err == nil {
		t.Error("expected error for empty user name")
	}
}

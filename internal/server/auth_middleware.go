package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/auth"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// contextKey is an unexported type used for context keys in this package.
type contextKey string

const (
	// jwtClaimsContextKey is the context key for JWT user claims.
	jwtClaimsContextKey contextKey = "jwtClaims"
	// userNameContextKey is the context key for the authenticated user name.
	userNameContextKey contextKey = "userName"
)

// isOIDCPublicPath returns true if the path is an OIDC endpoint that should
// be excluded from auth middleware (pre-auth endpoints).
func isOIDCPublicPath(path string) bool {
	switch path {
	case "/api/v1/auth/oidc/login",
		"/api/v1/auth/oidc/callback",
		"/api/v1/auth/token/refresh":
		return true
	}
	return false
}

// AuthMiddleware returns an HTTP middleware that enforces RBAC and records
// audit entries. If rbac is nil, the middleware is a no-op (backward
// compatible: all requests are allowed without authentication).
//
// When jwtSvc is non-nil, the middleware also supports JWT authentication.
// JWT tokens are tried first; if JWT validation fails, the middleware
// falls back to legacy bearer token authentication.
func AuthMiddleware(rbac *auth.RBAC, audit *auth.AuditLog, jwtSvc ...*auth.JWTService) func(http.Handler) http.Handler {
	var jwt *auth.JWTService
	if len(jwtSvc) > 0 {
		jwt = jwtSvc[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If no RBAC is configured, allow all requests (backward compatible).
			if rbac == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Allow health and version endpoints without auth.
			if r.URL.Path == "/api/v1/health" || r.URL.Path == "/api/v1/version" {
				next.ServeHTTP(w, r)
				return
			}

			// Allow OIDC public endpoints without auth.
			if isOIDCPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Allow CORS preflight without auth.
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Extract Bearer token from Authorization header.
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				recordAudit(audit, "", r, "denied", "missing Authorization header")
				errorResponse(w, http.StatusUnauthorized, "missing Authorization header")
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				recordAudit(audit, "", r, "denied", "invalid Authorization format")
				errorResponse(w, http.StatusUnauthorized, "invalid Authorization header format, expected 'Bearer <token>'")
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				recordAudit(audit, "", r, "denied", "empty token")
				errorResponse(w, http.StatusUnauthorized, "empty token")
				return
			}

			// Determine resource and action from HTTP method + path.
			resource, action := mapRequestToPermission(r.Method, r.URL.Path)

			// Try JWT validation first if JWTService is configured.
			if jwt != nil {
				claims, err := jwt.ValidateToken(token)
				if err == nil {
					// JWT is valid. Check RBAC permissions using the role from claims.
					userName := claims.Email
					if userName == "" {
						userName = claims.Name
					}
					if userName == "" {
						userName = claims.UserID
					}

					// Try RBAC authorization by user name.
					if authErr := rbac.Authorize(userName, resource, action); authErr != nil {
						recordAudit(audit, userName, r, "denied", authErr.Error())
						errorResponse(w, http.StatusForbidden, authErr.Error())
						return
					}

					// Store JWT claims and user name in context.
					ctx := context.WithValue(r.Context(), jwtClaimsContextKey, claims)
					ctx = context.WithValue(ctx, userNameContextKey, userName)
					recordAudit(audit, userName, r, "success", "")
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				// JWT validation failed, fall through to legacy token auth.
			}

			// Legacy bearer token authorization.
			userName, err := rbac.AuthorizeByToken(token, resource, action)
			if err != nil {
				if userName == "" {
					// Token not found -> 401 Unauthorized.
					recordAudit(audit, "", r, "denied", err.Error())
					errorResponse(w, http.StatusUnauthorized, "invalid token")
					return
				}
				// User found but not authorized -> 403 Forbidden.
				recordAudit(audit, userName, r, "denied", err.Error())
				errorResponse(w, http.StatusForbidden, err.Error())
				return
			}

			// Record successful auth and continue.
			ctx := context.WithValue(r.Context(), userNameContextKey, userName)
			recordAudit(audit, userName, r, "success", "")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// mapRequestToPermission maps an HTTP method and URL path to a resource name
// and action string used by the RBAC system.
func mapRequestToPermission(method, path string) (resource, action string) {
	// Strip the /api/v1/ prefix.
	trimmed := strings.TrimPrefix(path, "/api/v1/")

	// Determine resource from the first path segment.
	parts := strings.SplitN(trimmed, "/", 3)
	switch {
	case strings.HasPrefix(trimmed, "sandboxes"):
		resource = "sandbox"
	case strings.HasPrefix(trimmed, "matrices"):
		resource = "matrix"
	case strings.HasPrefix(trimmed, "sessions"):
		resource = "session"
	case strings.HasPrefix(trimmed, "pools"):
		resource = "pool"
	case strings.HasPrefix(trimmed, "blueprints"):
		resource = "blueprint"
	case strings.HasPrefix(trimmed, "teams"):
		resource = "team"
	case strings.HasPrefix(trimmed, "auth"):
		resource = "user"
	default:
		resource = parts[0]
	}

	// Special case: WebSocket exec stream requires exec permission.
	if method == http.MethodGet && len(parts) >= 3 && parts[2] == "exec" {
		return resource, "exec"
	}

	// Determine action from HTTP method and sub-path.
	switch method {
	case http.MethodGet:
		action = "read"
	case http.MethodPost:
		// Check for action sub-paths like /exec, /start, /stop.
		if len(parts) >= 3 {
			sub := parts[2]
			switch sub {
			case "exec":
				action = "exec"
			case "start", "stop":
				action = "update"
			case "snapshots":
				action = "create"
			case "end":
				action = "update"
			default:
				action = "create"
			}
		} else {
			action = "create"
		}
	case http.MethodPut, http.MethodPatch:
		action = "update"
	case http.MethodDelete:
		action = "delete"
	default:
		action = "read"
	}

	return resource, action
}

// recordAudit records an audit entry if the audit log is configured.
func recordAudit(audit *auth.AuditLog, userName string, r *http.Request, result, detail string) {
	if audit == nil {
		return
	}

	resource, action := mapRequestToPermission(r.Method, r.URL.Path)

	// Build a more descriptive resource path.
	resourcePath := resource
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) >= 2 && parts[1] != "" {
		resourcePath = resource + "/" + parts[1]
	}

	audit.Record(&v1alpha1.AuditEntry{
		Timestamp: time.Now(),
		User:      userName,
		Action:    resource + "." + action,
		Resource:  resourcePath,
		Result:    result,
		Detail:    detail,
	})
}

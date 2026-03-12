package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/auth"
	v1alpha1 "github.com/hg-dendi/sandboxmatrix/pkg/api/v1alpha1"
)

// stateStore is a simple in-memory store for OIDC state parameters to
// prevent CSRF attacks. States expire after 10 minutes.
type stateStore struct {
	mu     sync.Mutex
	states map[string]time.Time
}

func newStateStore() *stateStore {
	return &stateStore{states: make(map[string]time.Time)}
}

// generate creates a random state string, stores it, and returns it.
func (s *stateStore) generate() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean expired states.
	now := time.Now()
	for k, v := range s.states {
		if now.After(v) {
			delete(s.states, k)
		}
	}

	s.states[state] = now.Add(10 * time.Minute)
	return state, nil
}

// validate checks if a state exists and removes it (one-time use).
func (s *stateStore) validate(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiry, ok := s.states[state]
	if !ok {
		return false
	}
	delete(s.states, state)
	return time.Now().Before(expiry)
}

// OIDCHandlerConfig holds the dependencies for OIDC handlers.
type OIDCHandlerConfig struct {
	Provider    *auth.OIDCProvider
	JWTService  *auth.JWTService
	RBAC        *auth.RBAC
	RoleMapping map[string]string // OIDC group -> sandboxmatrix role
}

// handleOIDCLogin redirects the user to the OIDC provider for authentication.
// Browser clients will be redirected via HTTP 302. API clients that send
// Accept: application/json will receive the redirect URL as JSON instead.
func handleOIDCLogin(oidcProvider *auth.OIDCProvider, states *stateStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := states.generate()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "failed to generate state")
			return
		}

		url := oidcProvider.AuthURL(state)

		// If the client prefers JSON, return the URL instead of redirecting.
		if r.Header.Get("Accept") == "application/json" {
			jsonResponse(w, http.StatusOK, map[string]string{
				"redirectUrl": url,
			})
			return
		}

		// Browser clients get a standard HTTP redirect.
		http.Redirect(w, r, url, http.StatusFound)
	}
}

// handleOIDCCallback handles the OIDC callback, exchanges the auth code for
// tokens, creates or updates the user in RBAC, and returns JWT tokens.
func handleOIDCCallback(cfg *OIDCHandlerConfig, states *stateStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate state parameter.
		state := r.URL.Query().Get("state")
		if state == "" || !states.validate(state) {
			errorResponse(w, http.StatusBadRequest, "invalid or expired state parameter")
			return
		}

		// Check for errors from the provider.
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			desc := r.URL.Query().Get("error_description")
			errorResponse(w, http.StatusBadRequest, "oidc error: "+errParam+": "+desc)
			return
		}

		// Get the authorization code.
		code := r.URL.Query().Get("code")
		if code == "" {
			errorResponse(w, http.StatusBadRequest, "missing authorization code")
			return
		}

		ctx := r.Context()

		// Exchange code for tokens.
		oidcTokens, err := cfg.Provider.Exchange(ctx, code)
		if err != nil {
			slog.Error("oidc: exchange failed", "error", err)
			errorResponse(w, http.StatusInternalServerError, "failed to exchange authorization code")
			return
		}

		// Get user information.
		var claims *auth.OIDCClaims
		if oidcTokens.IDToken != "" {
			// For standard OIDC providers, verify the ID token.
			claims, err = cfg.Provider.VerifyIDToken(ctx, oidcTokens.IDToken)
			if err != nil {
				slog.Error("oidc: verify id token failed", "error", err)
				errorResponse(w, http.StatusInternalServerError, "failed to verify ID token")
				return
			}
		} else {
			// For GitHub and fallback, use the userinfo/API endpoint.
			claims, err = cfg.Provider.GetUserInfo(ctx, oidcTokens.AccessToken)
			if err != nil {
				slog.Error("oidc: get user info failed", "error", err)
				errorResponse(w, http.StatusInternalServerError, "failed to get user info")
				return
			}
		}

		// Determine the user's role based on OIDC groups and role mapping.
		role := mapOIDCGroupsToRole(claims.Groups, cfg.RoleMapping)

		// Determine user identity. Prefer email, fall back to name or subject.
		userName := claims.Email
		if userName == "" {
			userName = claims.Name
		}
		if userName == "" {
			userName = claims.Subject
		}

		// Create or update user in RBAC.
		if cfg.RBAC != nil {
			ensureRBACUser(cfg.RBAC, userName, role)
		}

		// Generate JWT tokens.
		userClaims := auth.UserClaims{
			UserID: claims.Subject,
			Email:  claims.Email,
			Name:   claims.Name,
			Role:   string(role),
			Teams:  claims.Groups,
		}

		accessToken, err := cfg.JWTService.GenerateAccessToken(userClaims)
		if err != nil {
			slog.Error("oidc: generate access token failed", "error", err)
			errorResponse(w, http.StatusInternalServerError, "failed to generate access token")
			return
		}

		refreshToken, err := cfg.JWTService.GenerateRefreshToken(claims.Subject)
		if err != nil {
			slog.Error("oidc: generate refresh token failed", "error", err)
			errorResponse(w, http.StatusInternalServerError, "failed to generate refresh token")
			return
		}

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"accessToken":  accessToken,
			"refreshToken": refreshToken,
			"tokenType":    "Bearer",
			"expiresIn":    3600, // 1 hour in seconds
			"user": map[string]interface{}{
				"id":     claims.Subject,
				"email":  claims.Email,
				"name":   claims.Name,
				"role":   string(role),
				"groups": claims.Groups,
			},
		})
	}
}

// handleTokenRefresh refreshes an access token using a refresh token.
func handleTokenRefresh(jwtSvc *auth.JWTService, rbac *auth.RBAC) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RefreshToken string `json:"refreshToken"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.RefreshToken == "" {
			errorResponse(w, http.StatusBadRequest, "refreshToken is required")
			return
		}

		// Validate the refresh token.
		userID, err := jwtSvc.ValidateRefreshToken(req.RefreshToken)
		if err != nil {
			errorResponse(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}

		// Look up the user to get current role/info.
		var userClaims auth.UserClaims
		userClaims.UserID = userID

		if rbac != nil {
			// Try to find the user by iterating. The user might be identified
			// by subject ID or email.
			users := rbac.ListUsers()
			for _, u := range users {
				if u.Name == userID {
					userClaims.Name = u.Name
					userClaims.Role = string(u.Role)
					break
				}
			}
		}

		accessToken, err := jwtSvc.GenerateAccessToken(userClaims)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "failed to generate access token")
			return
		}

		// Generate a new refresh token (token rotation).
		newRefreshToken, err := jwtSvc.GenerateRefreshToken(userID)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, "failed to generate refresh token")
			return
		}

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"accessToken":  accessToken,
			"refreshToken": newRefreshToken,
			"tokenType":    "Bearer",
			"expiresIn":    3600,
		})
	}
}

// handleUserInfo returns the current user's info from the JWT token.
func handleUserInfo(jwtSvc *auth.JWTService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The JWT claims should be in the context (set by auth middleware).
		claims, ok := r.Context().Value(jwtClaimsContextKey).(*auth.UserClaims)
		if !ok || claims == nil {
			errorResponse(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"id":    claims.UserID,
			"email": claims.Email,
			"name":  claims.Name,
			"role":  claims.Role,
			"teams": claims.Teams,
		})
	}
}

// mapOIDCGroupsToRole maps OIDC groups to a sandboxmatrix role using the
// configured role mapping. Returns the highest-privilege matching role,
// or "viewer" as default.
func mapOIDCGroupsToRole(groups []string, roleMapping map[string]string) v1alpha1.Role {
	if len(roleMapping) == 0 || len(groups) == 0 {
		return v1alpha1.RoleViewer
	}

	// Priority: admin > operator > viewer.
	rolePriority := map[string]int{
		string(v1alpha1.RoleAdmin):    3,
		string(v1alpha1.RoleOperator): 2,
		string(v1alpha1.RoleViewer):   1,
	}

	bestRole := v1alpha1.RoleViewer
	bestPriority := 1

	for _, group := range groups {
		if role, ok := roleMapping[group]; ok {
			if priority, ok := rolePriority[role]; ok && priority > bestPriority {
				bestRole = v1alpha1.Role(role)
				bestPriority = priority
			}
		}
	}

	return bestRole
}

// ensureRBACUser creates or updates a user in the RBAC system.
func ensureRBACUser(rbac *auth.RBAC, name string, role v1alpha1.Role) {
	// Try to get existing user.
	_, err := rbac.GetUser(name)
	if err != nil {
		// User doesn't exist, create them.
		user := &v1alpha1.User{
			Name: name,
			Role: role,
		}
		if addErr := rbac.AddUser(user); addErr != nil {
			slog.Warn("oidc: failed to auto-create user", "name", name, "error", addErr)
		} else {
			slog.Info("oidc: auto-created user", "name", name, "role", role)
		}
	}
	// If user exists, we keep their current role (don't overwrite).
}

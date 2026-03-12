// Package auth provides role-based access control and audit logging.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// OIDCConfig holds configuration for an OIDC provider.
type OIDCConfig struct {
	// Provider is the OIDC provider type: "google", "github", or "oidc" (generic).
	Provider string `yaml:"provider" json:"provider"`
	// Issuer is the OIDC issuer URL (used for discovery). Not needed for google/github.
	Issuer string `yaml:"issuer" json:"issuer"`
	// ClientID is the OAuth2 client ID.
	ClientID string `yaml:"clientId" json:"clientId"`
	// ClientSecret is the OAuth2 client secret.
	ClientSecret string `yaml:"clientSecret" json:"clientSecret"`
	// RedirectURL is the callback URL (e.g. http://localhost:8080/api/v1/auth/oidc/callback).
	RedirectURL string `yaml:"redirectUrl" json:"redirectUrl"`
	// Scopes are the OAuth2 scopes to request.
	Scopes []string `yaml:"scopes" json:"scopes"`
}

// OIDCClaims holds the claims extracted from an OIDC ID token or userinfo.
type OIDCClaims struct {
	Subject string                 `json:"sub"`
	Email   string                 `json:"email"`
	Name    string                 `json:"name"`
	Groups  []string               `json:"groups,omitempty"`
	Raw     map[string]interface{} `json:"raw,omitempty"`
}

// OIDCTokens holds the tokens returned from an OIDC exchange.
type OIDCTokens struct {
	AccessToken  string    `json:"accessToken"`
	IDToken      string    `json:"idToken,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

// OIDCProvider handles OIDC authentication flows.
type OIDCProvider struct {
	config   OIDCConfig
	oauth2   oauth2.Config
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
}

// NewOIDCProvider creates a new OIDCProvider. For generic OIDC and Google,
// it discovers endpoints via .well-known/openid-configuration. For GitHub,
// it uses the GitHub OAuth2 endpoints (GitHub is not a standard OIDC provider).
func NewOIDCProvider(cfg OIDCConfig) (*OIDCProvider, error) {
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("oidc: clientId is required")
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Errorf("oidc: clientSecret is required")
	}
	if cfg.RedirectURL == "" {
		return nil, fmt.Errorf("oidc: redirectUrl is required")
	}

	p := &OIDCProvider{config: cfg}

	switch cfg.Provider {
	case "github":
		return p.initGitHub(cfg)
	case "google":
		return p.initGoogle(cfg)
	case "oidc", "":
		return p.initGenericOIDC(cfg)
	default:
		return nil, fmt.Errorf("oidc: unsupported provider %q (use google, github, or oidc)", cfg.Provider)
	}
}

// initGitHub sets up the provider for GitHub OAuth2 (not standard OIDC).
func (p *OIDCProvider) initGitHub(cfg OIDCConfig) (*OIDCProvider, error) {
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"user:email", "read:org"}
	}

	p.oauth2 = oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       scopes,
		Endpoint:     github.Endpoint,
	}
	// No OIDC verifier for GitHub; we use the /user API instead.
	return p, nil
}

// initGoogle sets up the provider for Google OIDC.
func (p *OIDCProvider) initGoogle(cfg OIDCConfig) (*OIDCProvider, error) {
	ctx := context.Background()

	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, fmt.Errorf("oidc: discover google endpoints: %w", err)
	}
	p.provider = provider

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	p.oauth2 = oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       scopes,
		Endpoint:     provider.Endpoint(),
	}

	p.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	return p, nil
}

// initGenericOIDC sets up a generic OIDC provider using the issuer URL.
func (p *OIDCProvider) initGenericOIDC(cfg OIDCConfig) (*OIDCProvider, error) {
	if cfg.Issuer == "" {
		return nil, fmt.Errorf("oidc: issuer is required for generic OIDC provider")
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: discover endpoints for %q: %w", cfg.Issuer, err)
	}
	p.provider = provider

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	p.oauth2 = oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       scopes,
		Endpoint:     provider.Endpoint(),
	}

	p.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	return p, nil
}

// AuthURL generates the authorization URL that the user should be redirected to.
// The state parameter should be a random string to prevent CSRF attacks.
func (p *OIDCProvider) AuthURL(state string) string {
	return p.oauth2.AuthCodeURL(state)
}

// Exchange exchanges an authorization code for tokens.
func (p *OIDCProvider) Exchange(ctx context.Context, code string) (*OIDCTokens, error) {
	token, err := p.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oidc: exchange code: %w", err)
	}

	result := &OIDCTokens{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
	}

	// Extract ID token if present (not available for GitHub).
	rawIDToken, ok := token.Extra("id_token").(string)
	if ok {
		result.IDToken = rawIDToken
	}

	return result, nil
}

// VerifyIDToken validates an ID token and extracts claims. This only works
// for standard OIDC providers (not GitHub).
func (p *OIDCProvider) VerifyIDToken(ctx context.Context, rawIDToken string) (*OIDCClaims, error) {
	if p.verifier == nil {
		return nil, fmt.Errorf("oidc: ID token verification not supported for provider %q", p.config.Provider)
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: verify ID token: %w", err)
	}

	claims := &OIDCClaims{
		Raw: make(map[string]interface{}),
	}

	if err := idToken.Claims(claims); err != nil {
		return nil, fmt.Errorf("oidc: extract claims: %w", err)
	}

	// Also extract all raw claims.
	if err := idToken.Claims(&claims.Raw); err != nil {
		return nil, fmt.Errorf("oidc: extract raw claims: %w", err)
	}

	claims.Subject = idToken.Subject

	return claims, nil
}

// GetUserInfo fetches user info using the access token. For GitHub, this
// calls the /user API. For OIDC providers, this calls the userinfo endpoint.
func (p *OIDCProvider) GetUserInfo(ctx context.Context, accessToken string) (*OIDCClaims, error) {
	if p.config.Provider == "github" {
		return p.getGitHubUserInfo(ctx, accessToken)
	}
	return p.getOIDCUserInfo(ctx, accessToken)
}

// getGitHubUserInfo fetches user info from the GitHub API.
func (p *OIDCProvider) getGitHubUserInfo(ctx context.Context, accessToken string) (*OIDCClaims, error) {
	// Fetch /user.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("oidc: create github user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc: github user request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("oidc: read github response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc: github /user returned %d: %s", resp.StatusCode, string(body))
	}

	var ghUser struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &ghUser); err != nil {
		return nil, fmt.Errorf("oidc: parse github user: %w", err)
	}

	claims := &OIDCClaims{
		Subject: fmt.Sprintf("%d", ghUser.ID),
		Email:   ghUser.Email,
		Name:    ghUser.Name,
		Raw:     make(map[string]interface{}),
	}
	// Use login as name fallback.
	if claims.Name == "" {
		claims.Name = ghUser.Login
	}
	_ = json.Unmarshal(body, &claims.Raw)

	// Fetch organizations for group mapping.
	orgs, err := p.getGitHubOrgs(ctx, accessToken)
	if err == nil {
		claims.Groups = orgs
	}

	return claims, nil
}

// getGitHubOrgs fetches user's GitHub organization memberships.
func (p *OIDCProvider) getGitHubOrgs(ctx context.Context, accessToken string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/orgs", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github /user/orgs returned %d", resp.StatusCode)
	}

	var orgs []struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &orgs); err != nil {
		return nil, err
	}

	result := make([]string, len(orgs))
	for i, org := range orgs {
		result[i] = org.Login
	}
	return result, nil
}

// getOIDCUserInfo fetches user info from the standard OIDC userinfo endpoint.
func (p *OIDCProvider) getOIDCUserInfo(ctx context.Context, accessToken string) (*OIDCClaims, error) {
	if p.provider == nil {
		return nil, fmt.Errorf("oidc: provider not initialized")
	}

	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	userInfo, err := p.provider.UserInfo(ctx, tokenSource)
	if err != nil {
		return nil, fmt.Errorf("oidc: fetch userinfo: %w", err)
	}

	claims := &OIDCClaims{
		Subject: userInfo.Subject,
		Email:   userInfo.Email,
		Raw:     make(map[string]interface{}),
	}

	if err := userInfo.Claims(claims); err != nil {
		return nil, fmt.Errorf("oidc: extract userinfo claims: %w", err)
	}
	if err := userInfo.Claims(&claims.Raw); err != nil {
		return nil, fmt.Errorf("oidc: extract raw userinfo claims: %w", err)
	}

	return claims, nil
}

// ProviderName returns the configured provider name.
func (p *OIDCProvider) ProviderName() string {
	return p.config.Provider
}

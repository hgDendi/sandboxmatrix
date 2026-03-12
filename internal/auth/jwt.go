package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTConfig holds configuration for the JWT token service.
type JWTConfig struct {
	// SigningKey is the HMAC-SHA256 signing key.
	SigningKey string `yaml:"signingKey" json:"signingKey"`
	// Issuer is the JWT issuer claim.
	Issuer string `yaml:"issuer" json:"issuer"`
	// Audience is the JWT audience claim.
	Audience string `yaml:"audience" json:"audience"`
	// AccessTokenTTL is the access token time-to-live (e.g. "1h").
	AccessTokenTTL time.Duration `yaml:"accessTokenTtl" json:"accessTokenTtl"`
	// RefreshTokenTTL is the refresh token time-to-live (e.g. "168h").
	RefreshTokenTTL time.Duration `yaml:"refreshTokenTtl" json:"refreshTokenTtl"`
}

// UserClaims holds the JWT claims for a sandboxMatrix user.
type UserClaims struct {
	jwt.RegisteredClaims
	UserID string   `json:"uid"`
	Email  string   `json:"email"`
	Name   string   `json:"name"`
	Role   string   `json:"role"`
	Teams  []string `json:"teams,omitempty"`
}

// JWTService provides JWT token generation and validation.
type JWTService struct {
	config     JWTConfig
	signingKey []byte
}

// NewJWTService creates a new JWTService. If SigningKey is empty, one is
// generated automatically. Default TTLs are 1h for access and 168h for refresh.
func NewJWTService(cfg JWTConfig) *JWTService {
	if cfg.AccessTokenTTL == 0 {
		cfg.AccessTokenTTL = time.Hour
	}
	if cfg.RefreshTokenTTL == 0 {
		cfg.RefreshTokenTTL = 168 * time.Hour // 7 days
	}
	if cfg.Issuer == "" {
		cfg.Issuer = "sandboxmatrix"
	}

	key := []byte(cfg.SigningKey)
	if len(key) == 0 {
		// Generate a random key if none provided.
		key = make([]byte, 32)
		_, _ = rand.Read(key)
	}

	return &JWTService{
		config:     cfg,
		signingKey: key,
	}
}

// GenerateAccessToken creates a signed JWT access token for the given user claims.
func (s *JWTService) GenerateAccessToken(claims UserClaims) (string, error) {
	now := time.Now()

	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    s.config.Issuer,
		Subject:   claims.UserID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.config.AccessTokenTTL)),
		NotBefore: jwt.NewNumericDate(now),
	}

	if s.config.Audience != "" {
		claims.RegisteredClaims.Audience = jwt.ClaimStrings{s.config.Audience}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.signingKey)
	if err != nil {
		return "", fmt.Errorf("jwt: sign access token: %w", err)
	}
	return signed, nil
}

// GenerateRefreshToken creates a signed JWT refresh token for the given user ID.
// Refresh tokens contain only the user ID and have a longer TTL.
func (s *JWTService) GenerateRefreshToken(userID string) (string, error) {
	now := time.Now()

	// Generate a unique token ID for the refresh token.
	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("jwt: generate refresh token id: %w", err)
	}

	claims := jwt.RegisteredClaims{
		Issuer:    s.config.Issuer,
		Subject:   userID,
		ID:        hex.EncodeToString(jtiBytes),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.config.RefreshTokenTTL)),
		NotBefore: jwt.NewNumericDate(now),
	}

	if s.config.Audience != "" {
		claims.Audience = jwt.ClaimStrings{s.config.Audience}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.signingKey)
	if err != nil {
		return "", fmt.Errorf("jwt: sign refresh token: %w", err)
	}
	return signed, nil
}

// ValidateToken validates a JWT token string and returns the user claims.
// It checks the signature, expiration, and issuer.
func (s *JWTService) ValidateToken(tokenString string) (*UserClaims, error) {
	claims := &UserClaims{}

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.config.Issuer),
	}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (interface{}, error) {
		return s.signingKey, nil
	}, parserOpts...)
	if err != nil {
		return nil, fmt.Errorf("jwt: validate token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("jwt: token is invalid")
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token and returns the subject (user ID).
func (s *JWTService) ValidateRefreshToken(tokenString string) (string, error) {
	claims := &jwt.RegisteredClaims{}

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.config.Issuer),
	}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (interface{}, error) {
		return s.signingKey, nil
	}, parserOpts...)
	if err != nil {
		return "", fmt.Errorf("jwt: validate refresh token: %w", err)
	}

	if !token.Valid {
		return "", fmt.Errorf("jwt: refresh token is invalid")
	}

	sub, err := claims.GetSubject()
	if err != nil {
		return "", fmt.Errorf("jwt: missing subject in refresh token: %w", err)
	}

	return sub, nil
}

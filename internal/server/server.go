package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/hg-dendi/sandboxmatrix/internal/agent/a2a"
	"github.com/hg-dendi/sandboxmatrix/internal/auth"
	"github.com/hg-dendi/sandboxmatrix/internal/autoscale"
	"github.com/hg-dendi/sandboxmatrix/internal/controller"
	"github.com/hg-dendi/sandboxmatrix/internal/quota"
	"github.com/hg-dendi/sandboxmatrix/internal/state"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server is an HTTP REST API server that exposes sandboxMatrix operations
// over HTTP so that SDKs and external clients can interact with the system
// without subprocess CLI calls.
type Server struct {
	ctrl         *controller.Controller
	gateway      *a2a.Gateway
	rbac         *auth.RBAC
	audit        *auth.AuditLog
	oidcProvider *auth.OIDCProvider
	jwtSvc       *auth.JWTService
	roleMapping  map[string]string
	teams        state.TeamStore
	quotaManager *quota.Manager
	autoscaler   *autoscale.Autoscaler
	addr         string
	router       *http.ServeMux
	server       *http.Server
}

// New creates a new Server that delegates to the given Controller and listens
// on addr (e.g. ":8080").
func New(ctrl *controller.Controller, addr string, opts ...Option) *Server {
	s := &Server{
		ctrl:   ctrl,
		addr:   addr,
		router: http.NewServeMux(),
	}
	for _, o := range opts {
		o(s)
	}
	s.registerRoutes()
	return s
}

// Option configures the Server.
type Option func(*Server)

// WithGateway attaches an A2A gateway for task sharding/aggregation endpoints.
func WithGateway(gw *a2a.Gateway) Option {
	return func(s *Server) { s.gateway = gw }
}

// WithRBAC enables RBAC authentication and audit logging on the server.
func WithRBAC(rbac *auth.RBAC, audit *auth.AuditLog) Option {
	return func(s *Server) { s.rbac = rbac; s.audit = audit }
}

// WithOIDC enables OIDC authentication on the server.
func WithOIDC(provider *auth.OIDCProvider, jwtSvc *auth.JWTService, roleMapping map[string]string) Option {
	return func(s *Server) {
		s.oidcProvider = provider
		s.jwtSvc = jwtSvc
		s.roleMapping = roleMapping
	}
}

// WithTeams enables team management and quota enforcement on the server.
func WithTeams(teams state.TeamStore, qm *quota.Manager) Option {
	return func(s *Server) { s.teams = teams; s.quotaManager = qm }
}

// WithAutoscaler enables dynamic resource autoscaling on the server.
func WithAutoscaler(as *autoscale.Autoscaler) Option {
	return func(s *Server) { s.autoscaler = as }
}

// registerRoutes sets up all API routes on the server's mux.
func (s *Server) registerRoutes() {
	// Health / Version / Metrics
	s.router.HandleFunc("GET /api/v1/health", handleHealth)
	s.router.HandleFunc("GET /api/v1/version", handleVersion)
	s.router.Handle("GET /metrics", promhttp.Handler())

	// Sandbox routes
	s.router.HandleFunc("POST /api/v1/sandboxes", handleCreateSandbox(s.ctrl))
	s.router.HandleFunc("GET /api/v1/sandboxes", handleListSandboxes(s.ctrl))
	s.router.HandleFunc("GET /api/v1/sandboxes/{name}", handleGetSandbox(s.ctrl))
	s.router.HandleFunc("POST /api/v1/sandboxes/{name}/start", handleStartSandbox(s.ctrl))
	s.router.HandleFunc("POST /api/v1/sandboxes/{name}/stop", handleStopSandbox(s.ctrl))
	s.router.HandleFunc("DELETE /api/v1/sandboxes/{name}", handleDestroySandbox(s.ctrl))
	s.router.HandleFunc("POST /api/v1/sandboxes/{name}/exec", handleExecSandbox(s.ctrl))
	s.router.HandleFunc("GET /api/v1/sandboxes/{name}/stats", handleStatsSandbox(s.ctrl))
	s.router.HandleFunc("POST /api/v1/sandboxes/{name}/snapshots", handleCreateSnapshot(s.ctrl))
	s.router.HandleFunc("GET /api/v1/sandboxes/{name}/snapshots", handleListSnapshots(s.ctrl))

	// Matrix routes
	s.router.HandleFunc("POST /api/v1/matrices", handleCreateMatrix(s.ctrl))
	s.router.HandleFunc("GET /api/v1/matrices", handleListMatrices(s.ctrl))
	s.router.HandleFunc("GET /api/v1/matrices/{name}", handleGetMatrix(s.ctrl))
	s.router.HandleFunc("POST /api/v1/matrices/{name}/start", handleStartMatrix(s.ctrl))
	s.router.HandleFunc("POST /api/v1/matrices/{name}/stop", handleStopMatrix(s.ctrl))
	s.router.HandleFunc("DELETE /api/v1/matrices/{name}", handleDestroyMatrix(s.ctrl))
	s.router.HandleFunc("POST /api/v1/matrices/{name}/shard", handleShardTask(s.ctrl, s.gateway))
	s.router.HandleFunc("POST /api/v1/matrices/{name}/collect", handleCollectResults(s.ctrl, s.gateway))

	// File operations
	s.router.HandleFunc("PUT /api/v1/sandboxes/{name}/files", handleUploadFile(s.ctrl))
	s.router.HandleFunc("GET /api/v1/sandboxes/{name}/files", handleDownloadFile(s.ctrl))
	s.router.HandleFunc("GET /api/v1/sandboxes/{name}/files/list", handleListFiles(s.ctrl))

	// WebSocket streaming exec
	s.router.HandleFunc("GET /api/v1/sandboxes/{name}/exec/stream", handleExecStream(s.ctrl))

	// Network / Port forwarding
	s.router.HandleFunc("GET /api/v1/sandboxes/{name}/ports", handleListPorts(s.ctrl))
	s.router.HandleFunc("GET /api/v1/matrices/{name}/services", handleListMatrixServices(s.ctrl))

	// Image building
	s.router.HandleFunc("POST /api/v1/images/build", handleBuildImage(s.ctrl))
	s.router.HandleFunc("GET /api/v1/images", handleListImages(s.ctrl))

	// Code interpreter
	s.router.HandleFunc("POST /api/v1/sandboxes/{name}/interpret", handleInterpret(s.ctrl))

	// Session routes
	s.router.HandleFunc("POST /api/v1/sessions", handleStartSession(s.ctrl))
	s.router.HandleFunc("GET /api/v1/sessions", handleListSessions(s.ctrl))
	s.router.HandleFunc("POST /api/v1/sessions/{id}/end", handleEndSession(s.ctrl))
	s.router.HandleFunc("POST /api/v1/sessions/{id}/exec", handleExecInSession(s.ctrl))

	// OIDC routes (only registered when OIDC is configured).
	if s.oidcProvider != nil && s.jwtSvc != nil {
		states := newStateStore()
		oidcCfg := &OIDCHandlerConfig{
			Provider:    s.oidcProvider,
			JWTService:  s.jwtSvc,
			RBAC:        s.rbac,
			RoleMapping: s.roleMapping,
		}
		s.router.HandleFunc("GET /api/v1/auth/oidc/login", handleOIDCLogin(s.oidcProvider, states))
		s.router.HandleFunc("GET /api/v1/auth/oidc/callback", handleOIDCCallback(oidcCfg, states))
		s.router.HandleFunc("POST /api/v1/auth/token/refresh", handleTokenRefresh(s.jwtSvc, s.rbac))
		s.router.HandleFunc("GET /api/v1/auth/userinfo", handleUserInfo(s.jwtSvc))
	}

	// Autoscale routes (only registered when autoscaler is configured).
	if s.autoscaler != nil {
		s.router.HandleFunc("GET /api/v1/autoscale/status", handleAutoscaleStatus(s.autoscaler))
		s.router.HandleFunc("POST /api/v1/autoscale/enable", handleAutoscaleEnable(s.autoscaler))
		s.router.HandleFunc("POST /api/v1/autoscale/disable", handleAutoscaleDisable(s.autoscaler))
		s.router.HandleFunc("PUT /api/v1/sandboxes/{name}/priority", handleSetPriority(s.autoscaler))
	}

	// Team routes (only registered when team store is configured).
	if s.teams != nil {
		s.router.HandleFunc("POST /api/v1/teams", handleCreateTeam(s.teams))
		s.router.HandleFunc("GET /api/v1/teams", handleListTeams(s.teams))
		s.router.HandleFunc("GET /api/v1/teams/{name}", handleGetTeam(s.teams))
		s.router.HandleFunc("PUT /api/v1/teams/{name}", handleUpdateTeam(s.teams))
		s.router.HandleFunc("DELETE /api/v1/teams/{name}", handleDeleteTeam(s.teams))
		s.router.HandleFunc("GET /api/v1/teams/{name}/members", handleListTeamMembers(s.teams))
		s.router.HandleFunc("POST /api/v1/teams/{name}/members", handleAddTeamMember(s.teams))
		s.router.HandleFunc("DELETE /api/v1/teams/{name}/members/{user}", handleRemoveTeamMember(s.teams))
		if s.quotaManager != nil {
			s.router.HandleFunc("GET /api/v1/teams/{name}/usage", handleGetTeamUsage(s.teams, s.quotaManager))
		}
	}
}

// Handler returns the fully middleware-wrapped http.Handler. This is useful
// for tests that want to use httptest.NewServer.
func (s *Server) Handler() http.Handler {
	handler := chainMiddleware(
		s.router,
		loggingMiddleware,
		corsMiddleware,
		jsonContentTypeMiddleware,
		AuthMiddleware(s.rbac, s.audit, s.jwtSvc),
	)
	// Wrap with OpenTelemetry HTTP instrumentation as the outermost layer
	// so that every request gets a trace span with W3C context propagation.
	return otelhttp.NewHandler(handler, "sandboxmatrix-http")
}

// Start begins listening and serving HTTP requests. It blocks until the
// server is shut down.
func (s *Server) Start() error {
	handler := s.Handler()

	s.server = &http.Server{
		Addr:              s.addr,
		Handler:           handler,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	slog.Info("sandboxMatrix API server starting", "addr", s.addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	slog.Info("shutting down API server")
	return s.server.Shutdown(ctx)
}

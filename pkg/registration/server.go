package registration

import (
	"context"
	"log/slog"

	registrationv1 "github.com/sivchari/keel/gen/keel/registration/v1"
)

const (
	// DefaultHeartbeatInterval is the default interval for provider heartbeats.
	DefaultHeartbeatInterval = 30
)

// Server implements the RegistrationService gRPC server.
type Server struct {
	registrationv1.UnimplementedRegistrationServiceServer
	registry          *Registry
	heartbeatInterval int32
	logger            *slog.Logger
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithHeartbeatInterval sets the heartbeat interval in seconds.
func WithHeartbeatInterval(seconds int32) ServerOption {
	return func(s *Server) {
		s.heartbeatInterval = seconds
	}
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// NewServer creates a new Registration server.
func NewServer(registry *Registry, opts ...ServerOption) *Server {
	s := &Server{
		registry:          registry,
		heartbeatInterval: DefaultHeartbeatInterval,
		logger:            slog.Default(),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.logger = s.logger.With("component", "registration-server")

	return s
}

// Register registers a provider with keel core.
func (s *Server) Register(ctx context.Context, req *registrationv1.RegisterRequest) (*registrationv1.RegisterResponse, error) {
	name := req.GetName()
	endpoint := req.GetEndpoint()
	version := req.GetVersion()
	supportsWatch := req.GetSupportsWatch()

	apiGroups := make([]string, 0, len(req.GetSupportedKinds()))
	seen := make(map[string]bool)

	for _, kind := range req.GetSupportedKinds() {
		apiGroup := kind.GetApiGroup()
		if !seen[apiGroup] {
			apiGroups = append(apiGroups, apiGroup)
			seen[apiGroup] = true
		}
	}

	s.logger.Info("provider registration request",
		"name", name,
		"endpoint", endpoint,
		"version", version,
		"apiGroups", apiGroups,
	)

	if err := s.registry.Register(ctx, name, endpoint, version, apiGroups, supportsWatch); err != nil {
		s.logger.Error("provider registration failed", "name", name, "error", err)

		return &registrationv1.RegisterResponse{
			Registered: false,
			Message:    err.Error(),
		}, nil
	}

	return &registrationv1.RegisterResponse{
		Registered:               true,
		Message:                  "registered successfully",
		HeartbeatIntervalSeconds: s.heartbeatInterval,
	}, nil
}

// Deregister removes a provider from keel core.
func (s *Server) Deregister(ctx context.Context, req *registrationv1.DeregisterRequest) (*registrationv1.DeregisterResponse, error) {
	name := req.GetName()

	s.logger.Info("provider deregistration request", "name", name)

	if err := s.registry.Deregister(name); err != nil {
		s.logger.Error("provider deregistration failed", "name", name, "error", err)

		return &registrationv1.DeregisterResponse{
			Deregistered: false,
			Message:      err.Error(),
		}, nil
	}

	return &registrationv1.DeregisterResponse{
		Deregistered: true,
		Message:      "deregistered successfully",
	}, nil
}

// Heartbeat keeps the registration alive.
func (s *Server) Heartbeat(ctx context.Context, req *registrationv1.HeartbeatRequest) (*registrationv1.HeartbeatResponse, error) {
	name := req.GetName()

	if err := s.registry.Heartbeat(name); err != nil {
		s.logger.Warn("heartbeat failed", "name", name, "error", err)

		return &registrationv1.HeartbeatResponse{
			Acknowledged: false,
		}, nil
	}

	return &registrationv1.HeartbeatResponse{
		Acknowledged: true,
	}, nil
}

package registration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	providerv1 "github.com/sivchari/keel/gen/keel/provider/v1"
	registrationv1 "github.com/sivchari/keel/gen/keel/registration/v1"
)

// Client is a registration client that providers use to register with keel core.
type Client struct {
	conn              *grpc.ClientConn
	client            registrationv1.RegistrationServiceClient
	name              string
	endpoint          string
	heartbeatInterval time.Duration
	heartbeatStop     chan struct{}
	logger            *slog.Logger
}

// ClientOptions configures a registration client.
type ClientOptions struct {
	// KeelAddr is the address of the keel registration server.
	KeelAddr string

	// ProviderName is the unique name for this provider instance.
	ProviderName string

	// ProviderEndpoint is the gRPC endpoint where this provider is listening.
	ProviderEndpoint string

	// Logger for client logs.
	Logger *slog.Logger
}

// NewClient creates a new registration client.
func NewClient(_ context.Context, opts ClientOptions) (*Client, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	conn, err := grpc.NewClient(opts.KeelAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to keel at %s: %w", opts.KeelAddr, err)
	}

	return &Client{
		conn:     conn,
		client:   registrationv1.NewRegistrationServiceClient(conn),
		name:     opts.ProviderName,
		endpoint: opts.ProviderEndpoint,
		logger:   opts.Logger.With("component", "registration-client"),
	}, nil
}

// Register registers this provider with keel core.
func (c *Client) Register(ctx context.Context, version string, kinds []*providerv1.KindInfo, supportsWatch bool) error {
	resp, err := c.client.Register(ctx, &registrationv1.RegisterRequest{
		Name:           c.name,
		Endpoint:       c.endpoint,
		Version:        version,
		SupportedKinds: kinds,
		SupportsWatch:  supportsWatch,
	})
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	if !resp.GetRegistered() {
		return fmt.Errorf("registration rejected: %s", resp.GetMessage())
	}

	c.logger.Info("registered with keel",
		"name", c.name,
		"endpoint", c.endpoint,
	)

	if resp.GetHeartbeatIntervalSeconds() > 0 {
		c.heartbeatInterval = time.Duration(resp.GetHeartbeatIntervalSeconds()) * time.Second

		c.heartbeatStop = make(chan struct{})
		go c.heartbeatLoop()
	}

	return nil
}

// Deregister removes this provider from keel core.
func (c *Client) Deregister(ctx context.Context) error {
	if c.heartbeatStop != nil {
		close(c.heartbeatStop)
	}

	resp, err := c.client.Deregister(ctx, &registrationv1.DeregisterRequest{
		Name: c.name,
	})
	if err != nil {
		return fmt.Errorf("deregistration failed: %w", err)
	}

	if !resp.GetDeregistered() {
		return fmt.Errorf("deregistration rejected: %s", resp.GetMessage())
	}

	c.logger.Info("deregistered from keel", "name", c.name)

	return nil
}

// Close closes the connection to keel.
func (c *Client) Close() error {
	if c.heartbeatStop != nil {
		close(c.heartbeatStop)
		c.heartbeatStop = nil
	}

	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("failed to close connection: %w", err)
	}

	return nil
}

func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.heartbeatStop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			resp, err := c.client.Heartbeat(ctx, &registrationv1.HeartbeatRequest{
				Name:    c.name,
				Healthy: true,
			})

			cancel()

			if err != nil {
				c.logger.Warn("heartbeat failed", "error", err)

				continue
			}

			if !resp.GetAcknowledged() {
				c.logger.Warn("heartbeat not acknowledged")
			}
		}
	}
}

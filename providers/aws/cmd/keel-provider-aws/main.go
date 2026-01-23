// Package main implements the AWS provider gRPC server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	providerv1 "github.com/sivchari/keel/gen/keel/provider/v1"
	"github.com/sivchari/keel/pkg/registration"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/sivchari/keel/providers/aws/internal/provider"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		addr         = flag.String("addr", ":50051", "server address")
		region       = flag.String("region", "", "AWS region (defaults to AWS_REGION env var)")
		endpoint     = flag.String("endpoint", "", "custom AWS endpoint URL (for LocalStack)")
		keelAddr     = flag.String("keel-addr", "", "keel registration server address (e.g., localhost:50050)")
		providerName = flag.String("provider-name", "aws-provider", "unique provider instance name")
	)

	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := loadAWSConfig(ctx, *region)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	var lc net.ListenConfig

	listener, err := lc.Listen(ctx, "tcp", *addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", *addr, err)
	}

	server := grpc.NewServer()
	providerService := provider.New(&cfg, *endpoint, logger)

	providerv1.RegisterProviderServiceServer(server, providerService)
	reflection.Register(server)

	// Start gRPC server in background
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting AWS provider server", "addr", *addr, "region", cfg.Region, "endpoint", *endpoint)
		serverErr <- server.Serve(listener)
	}()

	// Register with keel if address is provided
	var regClient *registration.Client
	if *keelAddr != "" {
		regClient, err = registration.NewClient(ctx, registration.ClientOptions{
			KeelAddr:         *keelAddr,
			ProviderName:     *providerName,
			ProviderEndpoint: *addr,
			Logger:           logger,
		})
		if err != nil {
			return fmt.Errorf("failed to create registration client: %w", err)
		}

		info, err := providerService.GetInfo(ctx, &providerv1.GetInfoRequest{})
		if err != nil {
			return fmt.Errorf("failed to get provider info: %w", err)
		}

		if err := regClient.Register(ctx, info.GetVersion(), info.GetSupportedKinds(), info.GetSupportsWatch()); err != nil {
			return fmt.Errorf("failed to register with keel: %w", err)
		}
	}

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		logger.Info("shutting down server")
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	}

	// Deregister from keel
	if regClient != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := regClient.Deregister(shutdownCtx); err != nil {
			logger.Warn("failed to deregister from keel", "error", err)
		}

		if err := regClient.Close(); err != nil {
			logger.Warn("failed to close registration client", "error", err)
		}
	}

	server.GracefulStop()

	return nil
}

func loadAWSConfig(ctx context.Context, region string) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error

	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load default config: %w", err)
	}

	return cfg, nil
}

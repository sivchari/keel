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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	providerv1 "github.com/sivchari/keel/gen/keel/provider/v1"
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
		addr     = flag.String("addr", ":50051", "server address")
		region   = flag.String("region", "", "AWS region (defaults to AWS_REGION env var)")
		endpoint = flag.String("endpoint", "", "custom AWS endpoint URL (for LocalStack)")
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

	go func() {
		<-ctx.Done()
		logger.Info("shutting down server")
		server.GracefulStop()
	}()

	logger.Info("starting AWS provider server", "addr", *addr, "region", cfg.Region, "endpoint", *endpoint)

	if err := server.Serve(listener); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

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

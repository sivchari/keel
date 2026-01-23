// Package main implements the keel CLI.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"

	registrationv1 "github.com/sivchari/keel/gen/keel/registration/v1"
	"github.com/sivchari/keel/pkg/controller"
	"github.com/sivchari/keel/pkg/provider"
	"github.com/sivchari/keel/pkg/reconciler"
	"github.com/sivchari/keel/pkg/registration"
	"github.com/sivchari/keel/pkg/source/git"
)

type config struct {
	repoPath         string
	pollInterval     time.Duration
	providers        string
	registrationAddr string
	workers          int
	logLevel         string
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := parseFlags()
	logger := setupLogger(cfg.logLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	gitSource, err := git.New(git.Options{
		Path:         cfg.repoPath,
		PollInterval: cfg.pollInterval,
		Logger:       logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create git source: %w", err)
	}

	registry := registration.NewRegistry(logger)

	defer func() {
		if err := registry.Close(); err != nil {
			logger.Error("failed to close registry", "error", err)
		}
	}()

	if cfg.providers != "" {
		if err := setupProviders(ctx, registry, cfg.providers, logger); err != nil {
			return fmt.Errorf("failed to setup providers: %w", err)
		}
	}

	if err := startRegistrationServer(ctx, registry, cfg.registrationAddr, logger); err != nil {
		return fmt.Errorf("failed to start registration server: %w", err)
	}

	return startController(ctx, gitSource, registry, &cfg, logger)
}

func parseFlags() config {
	var cfg config

	flag.StringVar(&cfg.repoPath, "repo", ".", "path to git repository")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 30*time.Second, "git polling interval")
	flag.StringVar(&cfg.providers, "providers", "", "comma-separated list of provider addresses (format: apiGroup=address)")
	flag.StringVar(&cfg.registrationAddr, "registration-addr", ":50050", "address for provider registration gRPC server")
	flag.IntVar(&cfg.workers, "workers", 1, "number of concurrent reconciliation workers")
	flag.StringVar(&cfg.logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	return cfg
}

func setupLogger(logLevel string) *slog.Logger {
	level := parseLogLevel(logLevel)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))

	slog.SetDefault(logger)

	return logger
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func startRegistrationServer(ctx context.Context, registry *registration.Registry, addr string, logger *slog.Logger) error {
	grpcServer := grpc.NewServer()
	regServer := registration.NewServer(registry, registration.WithLogger(logger))
	registrationv1.RegisterRegistrationServiceServer(grpcServer, regServer)

	var lc net.ListenConfig

	lis, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	go func() {
		logger.Info("starting registration server", "addr", addr)

		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("registration server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	return nil
}

func startController(ctx context.Context, gitSource *git.Source, registry *registration.Registry, cfg *config, logger *slog.Logger) error {
	rec := reconciler.New(reconciler.Options{
		Store:    gitSource,
		Registry: registry,
		Logger:   logger,
	})

	ctrl := controller.New("keel", rec, controller.Options{
		MaxConcurrentReconciles: cfg.workers,
		Logger:                  logger,
	})

	ctrl.Watch(gitSource)

	logger.Info("starting keel controller",
		"repo", cfg.repoPath,
		"pollInterval", cfg.pollInterval,
		"workers", cfg.workers,
		"registrationAddr", cfg.registrationAddr,
	)

	if err := ctrl.Start(ctx); err != nil {
		return fmt.Errorf("controller stopped with error: %w", err)
	}

	return nil
}

func setupProviders(ctx context.Context, registry *registration.Registry, providersFlag string, logger *slog.Logger) error {
	for _, p := range strings.Split(providersFlag, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid provider format: %s (expected apiGroup=address)", p)
		}

		apiGroup := strings.TrimSpace(parts[0])
		addr := strings.TrimSpace(parts[1])

		client, err := provider.NewClient(ctx, addr)
		if err != nil {
			return fmt.Errorf("failed to connect to provider %s at %s: %w", apiGroup, addr, err)
		}

		apiGroups := []string{apiGroup}

		for _, kind := range client.SupportedKinds() {
			if kind.GetApiGroup() != apiGroup {
				apiGroups = append(apiGroups, kind.GetApiGroup())
			}
		}

		if err := registry.RegisterWithClient(client.Name(), addr, client.Version(), apiGroups, false, client); err != nil {
			return fmt.Errorf("failed to register provider %s: %w", apiGroup, err)
		}

		logger.Info("connected to provider", "apiGroup", apiGroup, "name", client.Name(), "version", client.Version())
	}

	return nil
}

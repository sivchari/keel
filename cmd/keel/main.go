// Package main implements the keel CLI.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sivchari/keel/pkg/controller"
	"github.com/sivchari/keel/pkg/provider"
	"github.com/sivchari/keel/pkg/reconciler"
	"github.com/sivchari/keel/pkg/source/git"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		repoPath     = flag.String("repo", ".", "path to git repository")
		pollInterval = flag.Duration("poll-interval", 30*time.Second, "git polling interval")
		providers    = flag.String("providers", "", "comma-separated list of provider addresses (format: apiGroup=address)")
		workers      = flag.Int("workers", 1, "number of concurrent reconciliation workers")
		logLevel     = flag.String("log-level", "info", "log level (debug, info, warn, error)")
	)

	flag.Parse()

	level := parseLogLevel(*logLevel)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))

	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	gitSource, err := git.New(git.Options{
		Path:         *repoPath,
		PollInterval: *pollInterval,
		Logger:       logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create git source: %w", err)
	}

	rec := reconciler.New(reconciler.Options{
		Store:  gitSource,
		Logger: logger,
	})

	if *providers != "" {
		if err := setupProviders(ctx, rec, *providers, logger); err != nil {
			return fmt.Errorf("failed to setup providers: %w", err)
		}
	}

	ctrl := controller.New("keel", rec, controller.Options{
		MaxConcurrentReconciles: *workers,
		Logger:                  logger,
	})

	ctrl.Watch(gitSource)

	logger.Info("starting keel controller",
		"repo", *repoPath,
		"pollInterval", *pollInterval,
		"workers", *workers,
	)

	if err := ctrl.Start(ctx); err != nil {
		return fmt.Errorf("controller stopped with error: %w", err)
	}

	return nil
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

func setupProviders(ctx context.Context, rec *reconciler.Reconciler, providersFlag string, logger *slog.Logger) error {
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

		rec.AddProvider(apiGroup, client)
		logger.Info("connected to provider", "apiGroup", apiGroup, "name", client.Name(), "version", client.Version())
	}

	return nil
}

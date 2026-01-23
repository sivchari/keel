// Package reconciler provides a resource reconciler that works with providers.
package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/sivchari/keel/pkg/core"
	"github.com/sivchari/keel/pkg/provider"
)

// ResourceStore provides access to resource definitions.
type ResourceStore interface {
	// Get returns the resource with the given key, or nil if not found.
	Get(key core.ResourceKey) *core.Resource
}

// ProviderRegistry provides access to registered providers.
type ProviderRegistry interface {
	// GetByAPIGroup returns the provider client for the given API group.
	GetByAPIGroup(apiGroup string) (*provider.Client, bool)
}

// Reconciler reconciles resources using providers.
type Reconciler struct {
	store    ResourceStore
	registry ProviderRegistry
	logger   *slog.Logger
}

// Options configures a Reconciler.
type Options struct {
	// Store provides access to resource definitions.
	Store ResourceStore

	// Registry provides access to registered providers.
	// If nil, providers must be added manually via AddProvider.
	Registry ProviderRegistry

	// Logger for reconciliation logs.
	Logger *slog.Logger
}

// New creates a new Reconciler.
func New(opts Options) *Reconciler {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &Reconciler{
		store:    opts.Store,
		registry: opts.Registry,
		logger:   opts.Logger.With("component", "reconciler"),
	}
}

// Reconcile implements core.Reconciler.
func (r *Reconciler) Reconcile(ctx context.Context, req core.Request) (core.Result, error) {
	key := req.Key

	r.logger.Debug("reconciling", "key", key.String())

	resource := r.store.Get(key)

	apiGroup := extractAPIGroup(key.APIVersion)

	client, ok := r.registry.GetByAPIGroup(apiGroup)
	if !ok {
		r.logger.Warn("no provider for API group", "apiGroup", apiGroup)

		return core.Result{}, nil
	}

	if resource == nil {
		return r.reconcileDelete(ctx, client, key)
	}

	return r.reconcileCreateOrUpdate(ctx, client, resource)
}

func (r *Reconciler) reconcileDelete(ctx context.Context, client *provider.Client, key core.ResourceKey) (core.Result, error) {
	result, err := client.Get(ctx, key)
	if err != nil {
		return core.Result{}, fmt.Errorf("failed to get resource state: %w", err)
	}

	if !result.Exists {
		r.logger.Debug("resource already deleted", "key", key.String())

		return core.Result{}, nil
	}

	r.logger.Info("deleting resource", "key", key.String())

	if err := client.Delete(ctx, key); err != nil {
		return core.Result{}, fmt.Errorf("failed to delete resource: %w", err)
	}

	r.logger.Info("resource deleted", "key", key.String())

	return core.Result{}, nil
}

func (r *Reconciler) reconcileCreateOrUpdate(ctx context.Context, client *provider.Client, resource *core.Resource) (core.Result, error) {
	key := resource.Key

	validateResult, err := client.Validate(ctx, resource)
	if err != nil {
		return core.Result{}, fmt.Errorf("failed to validate resource: %w", err)
	}

	if !validateResult.Valid {
		r.logger.Error("resource validation failed", "key", key.String(), "errors", validateResult.Errors)

		return core.Result{}, nil
	}

	result, err := client.Get(ctx, key)
	if err != nil {
		return core.Result{}, fmt.Errorf("failed to get resource state: %w", err)
	}

	if !result.Exists {
		r.logger.Info("creating resource", "key", key.String())

		if _, err := client.Create(ctx, resource); err != nil {
			return core.Result{}, fmt.Errorf("failed to create resource: %w", err)
		}

		r.logger.Info("resource created", "key", key.String())

		return core.Result{}, nil
	}

	r.logger.Info("updating resource", "key", key.String())

	if err := client.Update(ctx, resource); err != nil {
		return core.Result{}, fmt.Errorf("failed to update resource: %w", err)
	}

	r.logger.Info("resource updated", "key", key.String())

	return core.Result{}, nil
}

func extractAPIGroup(apiVersion string) string {
	parts := strings.Split(apiVersion, "/")
	if len(parts) > 0 {
		return parts[0]
	}

	return apiVersion
}

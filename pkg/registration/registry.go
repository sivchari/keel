// Package registration provides provider registration functionality.
package registration

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sivchari/keel/pkg/provider"
)

// ProviderInfo holds information about a registered provider.
type ProviderInfo struct {
	Name          string
	Endpoint      string
	Version       string
	APIGroups     []string
	SupportsWatch bool
	Client        *provider.Client
	RegisteredAt  time.Time
	LastHeartbeat time.Time
}

// Registry manages registered providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]*ProviderInfo // keyed by provider name
	apiGroups map[string]string        // api group -> provider name
	logger    *slog.Logger

	// OnRegister is called when a provider is registered.
	OnRegister func(info *ProviderInfo)

	// OnDeregister is called when a provider is deregistered.
	OnDeregister func(name string)
}

// NewRegistry creates a new provider registry.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}

	return &Registry{
		providers: make(map[string]*ProviderInfo),
		apiGroups: make(map[string]string),
		logger:    logger.With("component", "registry"),
	}
}

// Register registers a provider and connects to it.
func (r *Registry) Register(ctx context.Context, name, endpoint, version string, apiGroups []string, supportsWatch bool) error {
	client, err := provider.NewClient(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("failed to connect to provider %q at %s: %w", name, endpoint, err)
	}

	return r.RegisterWithClient(name, endpoint, version, apiGroups, supportsWatch, client)
}

// RegisterWithClient registers a provider with an existing client connection.
func (r *Registry) RegisterWithClient(name, endpoint, version string, apiGroups []string, supportsWatch bool, client *provider.Client) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %q already registered", name)
	}

	for _, apiGroup := range apiGroups {
		if existingProvider, exists := r.apiGroups[apiGroup]; exists {
			return fmt.Errorf("API group %q already registered by provider %q", apiGroup, existingProvider)
		}
	}

	now := time.Now()
	info := &ProviderInfo{
		Name:          name,
		Endpoint:      endpoint,
		Version:       version,
		APIGroups:     apiGroups,
		SupportsWatch: supportsWatch,
		Client:        client,
		RegisteredAt:  now,
		LastHeartbeat: now,
	}

	r.providers[name] = info

	for _, apiGroup := range apiGroups {
		r.apiGroups[apiGroup] = name
	}

	r.logger.Info("provider registered",
		"name", name,
		"endpoint", endpoint,
		"version", version,
		"apiGroups", apiGroups,
	)

	if r.OnRegister != nil {
		r.OnRegister(info)
	}

	return nil
}

// Deregister removes a provider from the registry.
func (r *Registry) Deregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.providers[name]
	if !exists {
		return fmt.Errorf("provider %q not found", name)
	}

	if err := info.Client.Close(); err != nil {
		r.logger.Warn("failed to close provider connection", "name", name, "error", err)
	}

	for _, apiGroup := range info.APIGroups {
		delete(r.apiGroups, apiGroup)
	}

	delete(r.providers, name)

	r.logger.Info("provider deregistered", "name", name)

	if r.OnDeregister != nil {
		r.OnDeregister(name)
	}

	return nil
}

// Heartbeat updates the last heartbeat time for a provider.
func (r *Registry) Heartbeat(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.providers[name]
	if !exists {
		return fmt.Errorf("provider %q not found", name)
	}

	info.LastHeartbeat = time.Now()

	return nil
}

// GetByAPIGroup returns the provider client for the given API group.
func (r *Registry) GetByAPIGroup(apiGroup string) (*provider.Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providerName, exists := r.apiGroups[apiGroup]
	if !exists {
		return nil, false
	}

	info, exists := r.providers[providerName]
	if !exists {
		return nil, false
	}

	return info.Client, true
}

// GetByName returns the provider info by name.
func (r *Registry) GetByName(name string) (*ProviderInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.providers[name]
	if !exists {
		return nil, false
	}

	return info, true
}

// List returns all registered providers.
func (r *Registry) List() []*ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ProviderInfo, 0, len(r.providers))
	for _, info := range r.providers {
		result = append(result, info)
	}

	return result
}

// Close closes all provider connections.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error

	for name, info := range r.providers {
		if err := info.Client.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close provider %q: %w", name, err))
		}
	}

	r.providers = make(map[string]*ProviderInfo)
	r.apiGroups = make(map[string]string)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing providers: %v", errs)
	}

	return nil
}

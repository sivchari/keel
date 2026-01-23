// Package provider implements the AWS provider gRPC service.
package provider

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	providerv1 "github.com/sivchari/keel/gen/keel/provider/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/sivchari/keel/providers/aws/internal/s3"
)

const (
	providerName    = "aws"
	providerVersion = "0.1.0"
	apiGroup        = "aws.keel.dev"
)

// Provider implements the AWS provider service.
type Provider struct {
	providerv1.UnimplementedProviderServiceServer

	cfg       *aws.Config
	logger    *slog.Logger
	s3Handler *s3.Handler
}

// New creates a new AWS provider.
func New(cfg *aws.Config, logger *slog.Logger) *Provider {
	return &Provider{
		cfg:       cfg,
		logger:    logger.With("provider", providerName),
		s3Handler: s3.NewHandler(cfg),
	}
}

// GetInfo returns provider metadata.
func (p *Provider) GetInfo(_ context.Context, _ *providerv1.GetInfoRequest) (*providerv1.GetInfoResponse, error) {
	return &providerv1.GetInfoResponse{
		Name:    providerName,
		Version: providerVersion,
		SupportedKinds: []*providerv1.KindInfo{
			{
				ApiGroup: apiGroup,
				Kind:     "S3Bucket",
				Versions: []string{"v1alpha1"},
			},
		},
		SupportsWatch: false,
	}, nil
}

// Get retrieves the actual state of a resource.
func (p *Provider) Get(ctx context.Context, req *providerv1.GetRequest) (*providerv1.GetResponse, error) {
	key := req.GetKey()

	p.logger.Debug("get resource", "key", formatKey(key))

	handler, err := p.getHandler(key)
	if err != nil {
		return nil, err
	}

	resp, err := handler.Get(ctx, key)
	if err != nil {
		p.logger.Error("get failed", "key", formatKey(key), "error", err)

		return nil, status.Errorf(codes.Internal, "failed to get resource: %v", err)
	}

	return resp, nil
}

// List retrieves all resources of a given kind.
func (p *Provider) List(_ context.Context, req *providerv1.ListRequest) (*providerv1.ListResponse, error) {
	p.logger.Debug("list resources", "kind", req.GetKind())

	return nil, fmt.Errorf("list not implemented: %w", status.Error(codes.Unimplemented, "List not implemented"))
}

// Create creates a new resource.
func (p *Provider) Create(ctx context.Context, req *providerv1.CreateRequest) (*providerv1.CreateResponse, error) {
	resource := req.GetResource()
	key := resource.GetKey()

	p.logger.Info("create resource", "key", formatKey(key))

	handler, err := p.getHandler(key)
	if err != nil {
		return nil, err
	}

	resp, err := handler.Create(ctx, resource)
	if err != nil {
		p.logger.Error("create failed", "key", formatKey(key), "error", err)

		return nil, status.Errorf(codes.Internal, "failed to create resource: %v", err)
	}

	p.logger.Info("resource created", "key", formatKey(key))

	return resp, nil
}

// Update updates an existing resource.
func (p *Provider) Update(ctx context.Context, req *providerv1.UpdateRequest) (*providerv1.UpdateResponse, error) {
	resource := req.GetResource()
	key := resource.GetKey()

	p.logger.Info("update resource", "key", formatKey(key))

	handler, err := p.getHandler(key)
	if err != nil {
		return nil, err
	}

	resp, err := handler.Update(ctx, resource)
	if err != nil {
		p.logger.Error("update failed", "key", formatKey(key), "error", err)

		return nil, status.Errorf(codes.Internal, "failed to update resource: %v", err)
	}

	p.logger.Info("resource updated", "key", formatKey(key))

	return resp, nil
}

// Delete deletes a resource.
func (p *Provider) Delete(ctx context.Context, req *providerv1.DeleteRequest) (*providerv1.DeleteResponse, error) {
	key := req.GetKey()

	p.logger.Info("delete resource", "key", formatKey(key))

	handler, err := p.getHandler(key)
	if err != nil {
		return nil, err
	}

	resp, err := handler.Delete(ctx, key)
	if err != nil {
		p.logger.Error("delete failed", "key", formatKey(key), "error", err)

		return nil, status.Errorf(codes.Internal, "failed to delete resource: %v", err)
	}

	p.logger.Info("resource deleted", "key", formatKey(key))

	return resp, nil
}

// Validate validates a resource definition.
func (p *Provider) Validate(_ context.Context, req *providerv1.ValidateRequest) (*providerv1.ValidateResponse, error) {
	resource := req.GetResource()
	key := resource.GetKey()

	p.logger.Debug("validate resource", "key", formatKey(key))

	handler, err := p.getHandler(key)
	if err != nil {
		return nil, err
	}

	return handler.Validate(resource), nil
}

// WatchEvents is not implemented for AWS provider.
func (p *Provider) WatchEvents(_ *providerv1.WatchEventsRequest, _ providerv1.ProviderService_WatchEventsServer) error {
	return fmt.Errorf("watch events not supported: %w", status.Error(codes.Unimplemented, "WatchEvents not supported"))
}

// ResourceHandler defines the interface for resource-specific handlers.
type ResourceHandler interface {
	Get(ctx context.Context, key *providerv1.ResourceKey) (*providerv1.GetResponse, error)
	Create(ctx context.Context, resource *providerv1.Resource) (*providerv1.CreateResponse, error)
	Update(ctx context.Context, resource *providerv1.Resource) (*providerv1.UpdateResponse, error)
	Delete(ctx context.Context, key *providerv1.ResourceKey) (*providerv1.DeleteResponse, error)
	Validate(resource *providerv1.Resource) *providerv1.ValidateResponse
}

func (p *Provider) getHandler(key *providerv1.ResourceKey) (ResourceHandler, error) {
	switch key.GetKind() {
	case "S3Bucket":
		return p.s3Handler, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported kind: %s", key.GetKind())
	}
}

func formatKey(key *providerv1.ResourceKey) string {
	if ns := key.GetNamespace(); ns != "" {
		return fmt.Sprintf("%s/%s/%s/%s", key.GetApiVersion(), key.GetKind(), ns, key.GetName())
	}

	return fmt.Sprintf("%s/%s/%s", key.GetApiVersion(), key.GetKind(), key.GetName())
}

// Package provider provides a gRPC client for communicating with providers.
package provider

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"

	providerv1 "github.com/sivchari/keel/gen/keel/provider/v1"
	"github.com/sivchari/keel/pkg/core"
)

// Client wraps a gRPC provider client.
type Client struct {
	conn   *grpc.ClientConn
	client providerv1.ProviderServiceClient
	info   *providerv1.GetInfoResponse
}

// NewClient creates a new provider client connected to the given address.
func NewClient(ctx context.Context, addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to provider at %s: %w", addr, err)
	}

	client := providerv1.NewProviderServiceClient(conn)

	info, err := client.GetInfo(ctx, &providerv1.GetInfoRequest{})
	if err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("failed to get provider info: %w", err)
	}

	return &Client{
		conn:   conn,
		client: client,
		info:   info,
	}, nil
}

// Close closes the connection to the provider.
func (c *Client) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("failed to close connection: %w", err)
	}

	return nil
}

// Name returns the provider name.
func (c *Client) Name() string {
	return c.info.GetName()
}

// Version returns the provider version.
func (c *Client) Version() string {
	return c.info.GetVersion()
}

// SupportedKinds returns the kinds supported by this provider.
func (c *Client) SupportedKinds() []*providerv1.KindInfo {
	return c.info.GetSupportedKinds()
}

// SupportsKind returns true if the provider supports the given API group and kind.
func (c *Client) SupportsKind(apiGroup, kind string) bool {
	for _, k := range c.info.GetSupportedKinds() {
		if k.GetApiGroup() == apiGroup && k.GetKind() == kind {
			return true
		}
	}

	return false
}

// Get retrieves the actual state of a resource.
func (c *Client) Get(ctx context.Context, key core.ResourceKey) (*GetResult, error) {
	resp, err := c.client.Get(ctx, &providerv1.GetRequest{
		Key: toProtoKey(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get failed: %w", err)
	}

	return &GetResult{
		Exists:      resp.GetExists(),
		ActualState: resp.GetActualState().AsMap(),
		Metadata:    resp.GetMetadata(),
	}, nil
}

// Create creates a new resource.
func (c *Client) Create(ctx context.Context, resource *core.Resource) (*CreateResult, error) {
	spec, err := structpb.NewStruct(resource.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to convert spec: %w", err)
	}

	resp, err := c.client.Create(ctx, &providerv1.CreateRequest{
		Resource: &providerv1.Resource{
			Key:  toProtoKey(resource.Key),
			Spec: spec,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create failed: %w", err)
	}

	return &CreateResult{
		Metadata: resp.GetMetadata(),
	}, nil
}

// Update updates an existing resource.
func (c *Client) Update(ctx context.Context, resource *core.Resource) error {
	spec, err := structpb.NewStruct(resource.Spec)
	if err != nil {
		return fmt.Errorf("failed to convert spec: %w", err)
	}

	_, err = c.client.Update(ctx, &providerv1.UpdateRequest{
		Resource: &providerv1.Resource{
			Key:  toProtoKey(resource.Key),
			Spec: spec,
		},
	})
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	return nil
}

// Delete deletes a resource.
func (c *Client) Delete(ctx context.Context, key core.ResourceKey) error {
	_, err := c.client.Delete(ctx, &providerv1.DeleteRequest{
		Key: toProtoKey(key),
	})
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	return nil
}

// Validate validates a resource definition.
func (c *Client) Validate(ctx context.Context, resource *core.Resource) (*ValidateResult, error) {
	spec, err := structpb.NewStruct(resource.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to convert spec: %w", err)
	}

	resp, err := c.client.Validate(ctx, &providerv1.ValidateRequest{
		Resource: &providerv1.Resource{
			Key:  toProtoKey(resource.Key),
			Spec: spec,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("validate failed: %w", err)
	}

	protoErrs := resp.GetErrors()
	errs := make([]ValidationError, 0, len(protoErrs))

	for _, e := range protoErrs {
		errs = append(errs, ValidationError{
			Code:    e.GetCode(),
			Field:   e.GetField(),
			Message: e.GetMessage(),
		})
	}

	return &ValidateResult{
		Valid:  resp.GetValid(),
		Errors: errs,
	}, nil
}

// GetResult contains the result of a Get operation.
type GetResult struct {
	Exists      bool
	ActualState map[string]any
	Metadata    map[string]string
}

// CreateResult contains the result of a Create operation.
type CreateResult struct {
	Metadata map[string]string
}

// ValidateResult contains the result of a Validate operation.
type ValidateResult struct {
	Valid  bool
	Errors []ValidationError
}

// ValidationError represents a validation error.
type ValidationError struct {
	Code    string
	Field   string
	Message string
}

func toProtoKey(key core.ResourceKey) *providerv1.ResourceKey {
	pk := &providerv1.ResourceKey{
		ApiVersion: key.APIVersion,
		Kind:       key.Kind,
		Name:       key.Name,
	}

	if key.Namespace != "" {
		pk.Namespace = &key.Namespace
	}

	return pk
}

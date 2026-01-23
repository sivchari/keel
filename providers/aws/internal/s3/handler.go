// Package s3 provides S3 bucket resource handling for the AWS provider.
package s3

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	providerv1 "github.com/sivchari/keel/gen/keel/provider/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// Handler manages S3 bucket resources.
type Handler struct {
	client *s3.Client
	region string
}

// NewHandler creates a new S3 handler.
func NewHandler(cfg *aws.Config) *Handler {
	return &Handler{
		client: s3.NewFromConfig(*cfg),
		region: cfg.Region,
	}
}

// Get retrieves the actual state of an S3 bucket.
func (h *Handler) Get(ctx context.Context, key *providerv1.ResourceKey) (*providerv1.GetResponse, error) {
	bucketName := key.GetName()

	_, err := h.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return &providerv1.GetResponse{Exists: false}, nil
		}

		return nil, fmt.Errorf("failed to head bucket: %w", err)
	}

	actualState, err := h.getBucketState(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket state: %w", err)
	}

	return &providerv1.GetResponse{
		Exists:      true,
		ActualState: actualState,
		Metadata: map[string]string{
			"arn":    fmt.Sprintf("arn:aws:s3:::%s", bucketName),
			"region": h.region,
		},
	}, nil
}

// Create creates a new S3 bucket.
func (h *Handler) Create(ctx context.Context, resource *providerv1.Resource) (*providerv1.CreateResponse, error) {
	bucketName := resource.GetKey().GetName()
	spec := resource.GetSpec().AsMap()

	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}

	if h.region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(h.region),
		}
	}

	_, err := h.client.CreateBucket(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	if err := h.applyBucketConfig(ctx, bucketName, spec); err != nil {
		return nil, fmt.Errorf("failed to apply bucket config: %w", err)
	}

	return &providerv1.CreateResponse{
		Metadata: map[string]string{
			"arn":    fmt.Sprintf("arn:aws:s3:::%s", bucketName),
			"region": h.region,
		},
	}, nil
}

// Update updates an existing S3 bucket.
func (h *Handler) Update(ctx context.Context, resource *providerv1.Resource) (*providerv1.UpdateResponse, error) {
	bucketName := resource.GetKey().GetName()
	spec := resource.GetSpec().AsMap()

	if err := h.applyBucketConfig(ctx, bucketName, spec); err != nil {
		return nil, fmt.Errorf("failed to apply bucket config: %w", err)
	}

	return &providerv1.UpdateResponse{}, nil
}

// Delete deletes an S3 bucket.
func (h *Handler) Delete(ctx context.Context, key *providerv1.ResourceKey) (*providerv1.DeleteResponse, error) {
	bucketName := key.GetName()

	_, err := h.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return &providerv1.DeleteResponse{}, nil
		}

		return nil, fmt.Errorf("failed to delete bucket: %w", err)
	}

	return &providerv1.DeleteResponse{}, nil
}

// Validate validates an S3 bucket resource definition.
func (h *Handler) Validate(resource *providerv1.Resource) *providerv1.ValidateResponse {
	var errs []*providerv1.ValidationError

	bucketName := resource.GetKey().GetName()
	if bucketName == "" {
		errs = append(errs, &providerv1.ValidationError{
			Code:    "REQUIRED",
			Field:   "metadata.name",
			Message: "bucket name is required",
		})
	}

	if len(bucketName) < 3 || len(bucketName) > 63 {
		errs = append(errs, &providerv1.ValidationError{
			Code:    "INVALID_LENGTH",
			Field:   "metadata.name",
			Message: "bucket name must be between 3 and 63 characters",
		})
	}

	return &providerv1.ValidateResponse{
		Valid:  len(errs) == 0,
		Errors: errs,
	}
}

func (h *Handler) getBucketState(ctx context.Context, bucketName string) (*structpb.Struct, error) {
	state := make(map[string]any)
	state["bucketName"] = bucketName

	versioningOutput, err := h.client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(bucketName),
	})
	if err == nil {
		state["versioning"] = map[string]any{
			"enabled": versioningOutput.Status == types.BucketVersioningStatusEnabled,
		}
	}

	result, err := structpb.NewStruct(state)
	if err != nil {
		return nil, fmt.Errorf("failed to create struct: %w", err)
	}

	return result, nil
}

func (h *Handler) applyBucketConfig(ctx context.Context, bucketName string, spec map[string]any) error {
	if versioning, ok := spec["versioning"].(map[string]any); ok {
		if enabled, ok := versioning["enabled"].(bool); ok && enabled {
			_, err := h.client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
				Bucket: aws.String(bucketName),
				VersioningConfiguration: &types.VersioningConfiguration{
					Status: types.BucketVersioningStatusEnabled,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to enable versioning: %w", err)
			}
		}
	}

	if tags, ok := spec["tags"].(map[string]any); ok {
		var tagSet []types.Tag

		for k, v := range tags {
			if strVal, ok := v.(string); ok {
				tagSet = append(tagSet, types.Tag{
					Key:   aws.String(k),
					Value: aws.String(strVal),
				})
			}
		}

		if len(tagSet) > 0 {
			_, err := h.client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{
				Bucket:  aws.String(bucketName),
				Tagging: &types.Tagging{TagSet: tagSet},
			})
			if err != nil {
				return fmt.Errorf("failed to set tags: %w", err)
			}
		}
	}

	return nil
}

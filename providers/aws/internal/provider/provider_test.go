package provider_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	providerv1 "github.com/sivchari/keel/gen/keel/provider/v1"

	"github.com/sivchari/keel/providers/aws/internal/provider"
)

func TestProvider_GetInfo(t *testing.T) {
	t.Parallel()

	cfg := aws.Config{Region: "ap-northeast-1"}
	p := provider.New(&cfg, "", newTestLogger())

	resp, err := p.GetInfo(context.Background(), &providerv1.GetInfoRequest{})
	if err != nil {
		t.Fatalf("GetInfo() error = %v", err)
	}

	if resp.Name != "aws" {
		t.Errorf("GetInfo() Name = %q, want %q", resp.Name, "aws")
	}

	if resp.Version != "0.1.0" {
		t.Errorf("GetInfo() Version = %q, want %q", resp.Version, "0.1.0")
	}

	if len(resp.SupportedKinds) == 0 {
		t.Error("GetInfo() SupportedKinds is empty")
	}

	var foundS3Bucket bool

	for _, kind := range resp.SupportedKinds {
		if kind.Kind == "S3Bucket" {
			foundS3Bucket = true

			break
		}
	}

	if !foundS3Bucket {
		t.Error("GetInfo() SupportedKinds does not contain S3Bucket")
	}
}

func TestProvider_Validate_Valid(t *testing.T) {
	t.Parallel()

	cfg := aws.Config{Region: "ap-northeast-1"}
	p := provider.New(&cfg, "", newTestLogger())

	resp, err := p.Validate(context.Background(), &providerv1.ValidateRequest{
		Resource: &providerv1.Resource{
			Key: &providerv1.ResourceKey{
				ApiVersion: "aws.keel.dev/v1alpha1",
				Kind:       "S3Bucket",
				Name:       "my-valid-bucket",
			},
		},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if !resp.Valid {
		t.Errorf("Validate() Valid = %v, want true", resp.Valid)
	}

	if len(resp.Errors) != 0 {
		t.Errorf("Validate() Errors = %v, want empty", resp.Errors)
	}
}

func TestProvider_Validate_EmptyName(t *testing.T) {
	t.Parallel()

	cfg := aws.Config{Region: "ap-northeast-1"}
	p := provider.New(&cfg, "", newTestLogger())

	resp, err := p.Validate(context.Background(), &providerv1.ValidateRequest{
		Resource: &providerv1.Resource{
			Key: &providerv1.ResourceKey{
				ApiVersion: "aws.keel.dev/v1alpha1",
				Kind:       "S3Bucket",
				Name:       "",
			},
		},
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if resp.Valid {
		t.Error("Validate() Valid = true, want false for empty name")
	}

	if len(resp.Errors) == 0 {
		t.Error("Validate() Errors is empty, want validation errors")
	}
}

func TestProvider_Validate_InvalidLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		bucketName string
		wantValid  bool
	}{
		{
			name:       "too short",
			bucketName: "ab",
			wantValid:  false,
		},
		{
			name:       "minimum valid length",
			bucketName: "abc",
			wantValid:  true,
		},
		{
			name:       "maximum valid length",
			bucketName: "abcdefghijklmnopqrstuvwxyz1234567890abcdefghijklmnopqrstuvwxyza",
			wantValid:  true,
		},
		{
			name:       "too long",
			bucketName: "abcdefghijklmnopqrstuvwxyz1234567890abcdefghijklmnopqrstuvwxyzab",
			wantValid:  false,
		},
	}

	cfg := aws.Config{Region: "ap-northeast-1"}
	p := provider.New(&cfg, "", newTestLogger())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := p.Validate(context.Background(), &providerv1.ValidateRequest{
				Resource: &providerv1.Resource{
					Key: &providerv1.ResourceKey{
						ApiVersion: "aws.keel.dev/v1alpha1",
						Kind:       "S3Bucket",
						Name:       tt.bucketName,
					},
				},
			})
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if resp.Valid != tt.wantValid {
				t.Errorf("Validate() Valid = %v, want %v", resp.Valid, tt.wantValid)
			}
		})
	}
}

func TestProvider_GetHandler_UnsupportedKind(t *testing.T) {
	t.Parallel()

	cfg := aws.Config{Region: "ap-northeast-1"}
	p := provider.New(&cfg, "", newTestLogger())

	_, err := p.Get(context.Background(), &providerv1.GetRequest{
		Key: &providerv1.ResourceKey{
			ApiVersion: "aws.keel.dev/v1alpha1",
			Kind:       "UnsupportedKind",
			Name:       "test",
		},
	})
	if err == nil {
		t.Error("Get() with unsupported kind should return error")
	}
}

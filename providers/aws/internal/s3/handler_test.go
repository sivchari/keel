package s3_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	providerv1 "github.com/sivchari/keel/gen/keel/provider/v1"

	"github.com/sivchari/keel/providers/aws/internal/s3"
)

func newTestHandler() *s3.Handler {
	cfg := aws.Config{Region: "ap-northeast-1"}

	return s3.NewHandler(&cfg, "")
}

func TestHandler_Validate_ValidBucket(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	resp := h.Validate(&providerv1.Resource{
		Key: &providerv1.ResourceKey{
			ApiVersion: "aws.keel.dev/v1alpha1",
			Kind:       "S3Bucket",
			Name:       "my-bucket-123",
		},
	})

	if !resp.Valid {
		t.Errorf("Validate() Valid = %v, want true", resp.Valid)
	}
}

func TestHandler_Validate_EmptyBucketName(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	resp := h.Validate(&providerv1.Resource{
		Key: &providerv1.ResourceKey{
			ApiVersion: "aws.keel.dev/v1alpha1",
			Kind:       "S3Bucket",
			Name:       "",
		},
	})

	if resp.Valid {
		t.Error("Validate() Valid = true, want false for empty name")
	}

	wantCodes := []string{"REQUIRED", "INVALID_LENGTH"}
	if len(resp.Errors) != len(wantCodes) {
		t.Errorf("Validate() len(Errors) = %d, want %d", len(resp.Errors), len(wantCodes))
	}
}

func TestHandler_Validate_BucketNameLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		bucket    string
		wantValid bool
	}{
		{"too short", "ab", false},
		{"minimum length", "abc", true},
		{"maximum length", "abcdefghijklmnopqrstuvwxyz1234567890abcdefghijklmnopqrstuvwxyza", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz1234567890abcdefghijklmnopqrstuvwxyzab", false},
	}

	h := newTestHandler()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := h.Validate(&providerv1.Resource{
				Key: &providerv1.ResourceKey{
					ApiVersion: "aws.keel.dev/v1alpha1",
					Kind:       "S3Bucket",
					Name:       tt.bucket,
				},
			})

			if resp.Valid != tt.wantValid {
				t.Errorf("Validate() Valid = %v, want %v", resp.Valid, tt.wantValid)
			}
		})
	}
}

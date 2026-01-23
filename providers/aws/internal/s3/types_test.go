package s3_test

import (
	"testing"

	"github.com/sivchari/keel/providers/aws/internal/s3"
)

func TestParseSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
	}{
		{
			name:    "nil spec",
			input:   nil,
			wantErr: false,
		},
		{
			name:    "empty spec",
			input:   map[string]any{},
			wantErr: false,
		},
		{
			name: "full spec",
			input: map[string]any{
				"versioning": map[string]any{
					"enabled": true,
				},
				"tags": map[string]any{
					"environment": "dev",
					"team":        "platform",
				},
			},
			wantErr: false,
		},
		{
			name: "versioning only",
			input: map[string]any{
				"versioning": map[string]any{
					"enabled": false,
				},
			},
			wantErr: false,
		},
		{
			name: "tags only",
			input: map[string]any{
				"tags": map[string]any{
					"key": "value",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec, err := s3.ParseSpec(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && spec == nil {
				t.Error("ParseSpec() returned nil spec without error")
			}
		})
	}
}

func TestParseSpec_Values(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"versioning": map[string]any{
			"enabled": true,
		},
		"tags": map[string]any{
			"environment": "production",
		},
	}

	spec, err := s3.ParseSpec(input)
	if err != nil {
		t.Fatalf("ParseSpec() error = %v", err)
	}

	if spec.Versioning == nil {
		t.Fatal("Versioning is nil")
	}

	if !spec.Versioning.Enabled {
		t.Error("Versioning.Enabled = false, want true")
	}

	if spec.Tags["environment"] != "production" {
		t.Errorf("Tags[environment] = %q, want %q", spec.Tags["environment"], "production")
	}
}

func TestS3BucketSpec_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    *s3.S3BucketSpec
		wantErr bool
	}{
		{
			name:    "valid empty spec",
			spec:    &s3.S3BucketSpec{},
			wantErr: false,
		},
		{
			name: "valid spec with tags",
			spec: &s3.S3BucketSpec{
				Tags: map[string]string{
					"env": "dev",
				},
			},
			wantErr: false,
		},
		{
			name: "too many tags",
			spec: &s3.S3BucketSpec{
				Tags: makeTags(51),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func makeTags(n int) map[string]string {
	tags := make(map[string]string, n)
	for i := range n {
		tags[string(rune('a'+i%26))+string(rune('0'+i/26))] = "value"
	}
	return tags
}

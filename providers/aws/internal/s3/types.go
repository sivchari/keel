// Package s3 provides S3 bucket resource handling for the AWS provider.
package s3

import (
	"encoding/json"
	"fmt"
)

// S3BucketSpec defines the desired state of an S3 bucket.
type S3BucketSpec struct {
	// Versioning configures bucket versioning.
	Versioning *VersioningConfig `json:"versioning,omitempty"`

	// Tags are key-value pairs to attach to the bucket.
	// AWS allows max 50 tags, key max 128 chars, value max 256 chars.
	//govalid:maxitems=50
	Tags map[string]string `json:"tags,omitempty"`
}

// VersioningConfig configures S3 bucket versioning.
type VersioningConfig struct {
	// Enabled enables versioning on the bucket.
	Enabled bool `json:"enabled"`
}

// ParseSpec converts a map[string]any to S3BucketSpec.
func ParseSpec(spec map[string]any) (*S3BucketSpec, error) {
	if spec == nil {
		return &S3BucketSpec{}, nil
	}

	data, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %w", err)
	}

	var s S3BucketSpec
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}

	return &s, nil
}

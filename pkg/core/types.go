// Package core defines the core interfaces and types for the keel reconciliation engine.
package core

import (
	"fmt"
	"maps"
	"time"
)

// ResourceKey uniquely identifies a resource.
type ResourceKey struct {
	// APIVersion is the full API version (e.g., "aws.keel.dev/v1alpha1").
	APIVersion string

	// Kind is the resource kind (e.g., "S3Bucket").
	Kind string

	// Namespace is the logical namespace (optional).
	Namespace string

	// Name is the resource name.
	Name string
}

// String returns a string representation of the ResourceKey.
func (k ResourceKey) String() string {
	if k.Namespace == "" {
		return fmt.Sprintf("%s/%s/%s", k.APIVersion, k.Kind, k.Name)
	}
	return fmt.Sprintf("%s/%s/%s/%s", k.APIVersion, k.Kind, k.Namespace, k.Name)
}

// Request represents a reconciliation request.
type Request struct {
	// Key identifies the resource to reconcile.
	Key ResourceKey
}

// Result represents the outcome of a reconciliation.
type Result struct {
	// Requeue indicates whether to requeue the request immediately.
	Requeue bool

	// RequeueAfter indicates when to requeue the request.
	// If set, takes precedence over Requeue.
	RequeueAfter time.Duration
}

// ShouldRequeue returns true if the request should be requeued.
func (r Result) ShouldRequeue() bool {
	return r.Requeue || r.RequeueAfter > 0
}

// Resource represents a resource with its desired and actual state.
type Resource struct {
	// Key uniquely identifies this resource.
	Key ResourceKey

	// Spec is the desired state of the resource.
	Spec map[string]any

	// Labels are key-value pairs for organization.
	Labels map[string]string

	// Annotations are key-value pairs for additional metadata.
	Annotations map[string]string

	// Generation is incremented when spec changes.
	Generation int64

	// SourcePath is the path to the source file (for debugging).
	SourcePath string
}

// DeepCopy returns a deep copy of the resource.
func (r *Resource) DeepCopy() *Resource {
	if r == nil {
		return nil
	}

	cp := &Resource{
		Key:        r.Key,
		Generation: r.Generation,
		SourcePath: r.SourcePath,
	}

	if r.Spec != nil {
		cp.Spec = deepCopyMap(r.Spec)
	}
	if r.Labels != nil {
		cp.Labels = make(map[string]string, len(r.Labels))
		maps.Copy(cp.Labels, r.Labels)
	}
	if r.Annotations != nil {
		cp.Annotations = make(map[string]string, len(r.Annotations))
		maps.Copy(cp.Annotations, r.Annotations)
	}

	return cp
}

// deepCopyMap creates a deep copy of a map[string]any.
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = deepCopyValue(v)
	}
	return cp
}

// deepCopyValue creates a deep copy of a value.
func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopyMap(val)
	case []any:
		cp := make([]any, len(val))
		for i, item := range val {
			cp[i] = deepCopyValue(item)
		}
		return cp
	default:
		return v
	}
}

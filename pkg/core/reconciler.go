package core

import "context"

// Reconciler reconciles a resource to match its desired state.
// This is the core interface that implements the reconciliation logic.
type Reconciler interface {
	// Reconcile performs the reconciliation for the given request.
	// It compares the desired state with the actual state and takes
	// necessary actions to converge them.
	//
	// Returns:
	// - Result: indicates whether and when to requeue the request
	// - error: if an error occurred during reconciliation
	Reconcile(ctx context.Context, req Request) (Result, error)
}

// ReconcilerFunc is an adapter to allow the use of ordinary functions as Reconcilers.
type ReconcilerFunc func(ctx context.Context, req Request) (Result, error)

// Reconcile implements the Reconciler interface.
func (f ReconcilerFunc) Reconcile(ctx context.Context, req Request) (Result, error) {
	return f(ctx, req)
}

// TypedReconciler is a generic reconciler that works with a specific resource type.
type TypedReconciler[T any] interface {
	// Reconcile performs reconciliation for a typed resource.
	Reconcile(ctx context.Context, resource T) (Result, error)
}

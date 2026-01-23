package core

import "context"

// Source produces events that trigger reconciliation.
// This is inspired by controller-runtime's Source interface.
type Source interface {
	// Start begins producing events and sending them to the handler.
	// It should block until the context is cancelled.
	Start(ctx context.Context, handler EventHandler, queue WorkQueue) error
}

// EventHandler handles events from a Source.
type EventHandler interface {
	// OnCreate is called when a resource is created.
	OnCreate(ctx context.Context, key ResourceKey)

	// OnUpdate is called when a resource is updated.
	OnUpdate(ctx context.Context, key ResourceKey)

	// OnDelete is called when a resource is deleted.
	OnDelete(ctx context.Context, key ResourceKey)
}

// EnqueueEventHandler creates an EventHandler that enqueues to the given queue.
func EnqueueEventHandler(queue WorkQueue) EventHandler {
	return &enqueueEventHandler{queue: queue}
}

type enqueueEventHandler struct {
	queue WorkQueue
}

func (h *enqueueEventHandler) OnCreate(_ context.Context, key ResourceKey) {
	h.queue.Add(key)
}

func (h *enqueueEventHandler) OnUpdate(_ context.Context, key ResourceKey) {
	h.queue.Add(key)
}

func (h *enqueueEventHandler) OnDelete(_ context.Context, key ResourceKey) {
	h.queue.Add(key)
}

// ChangeEvent represents a change to a resource definition.
type ChangeEvent struct {
	// Type indicates the kind of change.
	Type ChangeType

	// Resource is the affected resource.
	Resource *Resource
}

// ChangeType categorizes resource changes.
type ChangeType int

const (
	// ChangeTypeAdded indicates a new resource was added.
	ChangeTypeAdded ChangeType = iota

	// ChangeTypeModified indicates a resource was modified.
	ChangeTypeModified

	// ChangeTypeDeleted indicates a resource was deleted.
	ChangeTypeDeleted
)

// String returns a string representation of the ChangeType.
func (t ChangeType) String() string {
	switch t {
	case ChangeTypeAdded:
		return "Added"
	case ChangeTypeModified:
		return "Modified"
	case ChangeTypeDeleted:
		return "Deleted"
	default:
		return "Unknown"
	}
}

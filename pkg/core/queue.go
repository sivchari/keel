package core

import "time"

// WorkQueue is a queue that supports rate limiting and delayed requeuing.
// This is inspired by client-go's workqueue.
type WorkQueue interface {
	// Add adds an item to the queue.
	Add(key ResourceKey)

	// AddAfter adds an item to the queue after the specified duration.
	AddAfter(key ResourceKey, duration time.Duration)

	// AddRateLimited adds an item to the queue with rate limiting.
	// The delay increases with each failure (exponential backoff).
	AddRateLimited(key ResourceKey)

	// Get blocks until an item is available or the queue is shut down.
	// Returns the item and whether the queue is shutting down.
	Get() (key ResourceKey, shutdown bool)

	// Done marks an item as processed.
	// This must be called after Get() to allow the item to be requeued.
	Done(key ResourceKey)

	// Forget resets the rate limiter for an item.
	// Call this when an item is processed successfully.
	Forget(key ResourceKey)

	// Len returns the number of items in the queue.
	Len() int

	// ShutDown shuts down the queue.
	// After this, Get() will return immediately with shutdown=true.
	ShutDown()

	// ShuttingDown returns true if the queue is shutting down.
	ShuttingDown() bool
}

// RateLimiter calculates how long an item should wait before being retried.
type RateLimiter interface {
	// When returns how long the item should wait.
	When(key ResourceKey) time.Duration

	// Forget resets the rate limiter state for an item.
	Forget(key ResourceKey)

	// NumRequeues returns how many times an item has been requeued.
	NumRequeues(key ResourceKey) int
}

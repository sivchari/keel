// Package workqueue provides a rate-limited work queue for the reconciliation loop.
package workqueue

import (
	"sync"
	"time"

	"github.com/sivchari/keel/pkg/core"
)

// ExponentialBackoffRateLimiter implements exponential backoff for failed items.
type ExponentialBackoffRateLimiter struct {
	mu       sync.RWMutex
	failures map[core.ResourceKey]int
	base     time.Duration
	max      time.Duration
}

// NewExponentialBackoffRateLimiter creates a new rate limiter with exponential backoff.
// The delay starts at baseDelay and doubles with each failure, capped at maxDelay.
func NewExponentialBackoffRateLimiter(baseDelay, maxDelay time.Duration) *ExponentialBackoffRateLimiter {
	return &ExponentialBackoffRateLimiter{
		failures: make(map[core.ResourceKey]int),
		base:     baseDelay,
		max:      maxDelay,
	}
}

// When returns how long the item should wait before being retried.
// Each call increments the failure count.
func (r *ExponentialBackoffRateLimiter) When(key core.ResourceKey) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.failures[key]++
	failures := r.failures[key]

	// Calculate exponential backoff: base * 2^(failures-1)
	backoff := r.base
	for i := 1; i < failures; i++ {
		backoff *= 2
		if backoff > r.max {
			backoff = r.max

			break
		}
	}

	return backoff
}

// Forget resets the rate limiter state for an item.
func (r *ExponentialBackoffRateLimiter) Forget(key core.ResourceKey) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.failures, key)
}

// NumRequeues returns how many times an item has been requeued.
func (r *ExponentialBackoffRateLimiter) NumRequeues(key core.ResourceKey) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.failures[key]
}

// Ensure ExponentialBackoffRateLimiter implements core.RateLimiter.
var _ core.RateLimiter = (*ExponentialBackoffRateLimiter)(nil)

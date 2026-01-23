package workqueue_test

import (
	"testing"
	"time"

	"github.com/sivchari/keel/pkg/controller/workqueue"
	"github.com/sivchari/keel/pkg/core"
)

func TestExponentialBackoffRateLimiter(t *testing.T) {
	t.Parallel()

	baseDelay := 1 * time.Millisecond
	maxDelay := 10 * time.Millisecond

	limiter := workqueue.NewExponentialBackoffRateLimiter(baseDelay, maxDelay)

	key := core.ResourceKey{
		APIVersion: "test/v1",
		Kind:       "Test",
		Name:       "test-resource",
	}

	tests := []struct {
		name        string
		requeues    int
		wantMinimum time.Duration
		wantMaximum time.Duration
	}{
		{
			name:        "first failure",
			requeues:    0,
			wantMinimum: baseDelay,
			wantMaximum: baseDelay,
		},
		{
			name:        "second failure",
			requeues:    1,
			wantMinimum: 2 * baseDelay,
			wantMaximum: 2 * baseDelay,
		},
		{
			name:        "third failure",
			requeues:    2,
			wantMinimum: 4 * baseDelay,
			wantMaximum: 4 * baseDelay,
		},
		{
			name:        "capped at max",
			requeues:    10,
			wantMinimum: maxDelay,
			wantMaximum: maxDelay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset limiter state
			limiter.Forget(key)

			// Simulate requeues
			for range tt.requeues {
				limiter.When(key)
			}

			got := limiter.When(key)
			if got < tt.wantMinimum || got > tt.wantMaximum {
				t.Errorf("When() = %v, want between %v and %v", got, tt.wantMinimum, tt.wantMaximum)
			}
		})
	}
}

func TestExponentialBackoffRateLimiter_NumRequeues(t *testing.T) {
	t.Parallel()

	limiter := workqueue.NewExponentialBackoffRateLimiter(time.Millisecond, time.Second)

	key := core.ResourceKey{
		APIVersion: "test/v1",
		Kind:       "Test",
		Name:       "test-resource",
	}

	if got := limiter.NumRequeues(key); got != 0 {
		t.Errorf("NumRequeues() = %d, want 0", got)
	}

	limiter.When(key)

	if got := limiter.NumRequeues(key); got != 1 {
		t.Errorf("NumRequeues() = %d, want 1", got)
	}

	limiter.When(key)

	if got := limiter.NumRequeues(key); got != 2 {
		t.Errorf("NumRequeues() = %d, want 2", got)
	}

	limiter.Forget(key)

	if got := limiter.NumRequeues(key); got != 0 {
		t.Errorf("NumRequeues() after Forget = %d, want 0", got)
	}
}

func TestExponentialBackoffRateLimiter_Concurrent(t *testing.T) {
	t.Parallel()

	limiter := workqueue.NewExponentialBackoffRateLimiter(time.Millisecond, time.Second)

	key := core.ResourceKey{
		APIVersion: "test/v1",
		Kind:       "Test",
		Name:       "test-resource",
	}

	done := make(chan struct{})

	for range 10 {
		go func() {
			for range 100 {
				limiter.When(key)
				limiter.NumRequeues(key)
			}
			done <- struct{}{}
		}()
	}

	for range 10 {
		<-done
	}
}

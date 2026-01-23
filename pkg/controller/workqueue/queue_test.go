package workqueue_test

import (
	"sync"
	"testing"
	"time"

	"github.com/sivchari/keel/pkg/controller/workqueue"
	"github.com/sivchari/keel/pkg/core"
)

func newTestKey(name string) core.ResourceKey {
	return core.ResourceKey{
		APIVersion: "test/v1",
		Kind:       "Test",
		Name:       name,
	}
}

func TestQueue_AddAndGet(t *testing.T) {
	t.Parallel()

	q := workqueue.New()
	defer q.ShutDown()

	key := newTestKey("test-1")
	q.Add(key)

	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1", q.Len())
	}

	got, shutdown := q.Get()

	if shutdown {
		t.Error("Get() returned shutdown=true unexpectedly")
	}

	if got != key {
		t.Errorf("Get() = %v, want %v", got, key)
	}

	q.Done(key)
}

func TestQueue_Deduplication(t *testing.T) {
	t.Parallel()

	q := workqueue.New()
	defer q.ShutDown()

	key := newTestKey("test-1")

	// Add same key multiple times
	q.Add(key)
	q.Add(key)
	q.Add(key)

	// Should only have 1 item
	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (deduplication failed)", q.Len())
	}

	got, _ := q.Get()
	q.Done(got)

	// Queue should be empty now
	if q.Len() != 0 {
		t.Errorf("Len() = %d, want 0", q.Len())
	}
}

func TestQueue_ReaddWhileProcessing(t *testing.T) {
	t.Parallel()

	q := workqueue.New()
	defer q.ShutDown()

	key := newTestKey("test-1")
	q.Add(key)

	got, _ := q.Get()

	// Add while processing (before Done)
	q.Add(key)

	// Should not be in queue yet (being processed)
	if q.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (item being processed)", q.Len())
	}

	q.Done(got)

	// Now it should be re-added
	if q.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (re-added after Done)", q.Len())
	}
}

func TestQueue_AddAfter(t *testing.T) {
	t.Parallel()

	q := workqueue.New()
	defer q.ShutDown()

	key := newTestKey("test-1")
	delay := 50 * time.Millisecond

	start := time.Now()

	q.AddAfter(key, delay)

	// Should not be available immediately
	if q.Len() != 0 {
		t.Errorf("Len() = %d immediately after AddAfter, want 0", q.Len())
	}

	got, shutdown := q.Get()
	elapsed := time.Since(start)

	if shutdown {
		t.Error("Get() returned shutdown=true unexpectedly")
	}

	if got != key {
		t.Errorf("Get() = %v, want %v", got, key)
	}

	if elapsed < delay {
		t.Errorf("Get() returned after %v, want at least %v", elapsed, delay)
	}

	q.Done(key)
}

func TestQueue_ShutDown(t *testing.T) {
	t.Parallel()

	q := workqueue.New()

	key := newTestKey("test-1")
	q.Add(key)

	q.ShutDown()

	if !q.ShuttingDown() {
		t.Error("ShuttingDown() = false after ShutDown()")
	}

	// Get should return immediately with shutdown=true
	_, shutdown := q.Get()
	if !shutdown {
		t.Error("Get() returned shutdown=false after ShutDown()")
	}

	// Adding after shutdown should be no-op
	q.Add(newTestKey("test-2"))

	if q.Len() != 0 {
		t.Errorf("Len() = %d after shutdown, want 0", q.Len())
	}
}

//nolint:funlen // Concurrent test requires setup for producers, consumers, and verification.
func TestQueue_Concurrent(t *testing.T) {
	t.Parallel()

	q := workqueue.New()
	defer q.ShutDown()

	const numProducers = 5

	const numConsumers = 3

	const itemsPerProducer = 100

	var wg sync.WaitGroup

	processed := make(chan core.ResourceKey, numProducers*itemsPerProducer)

	// Producers
	for i := range numProducers {
		wg.Add(1)

		go func(producerID int) {
			defer wg.Done()

			for j := range itemsPerProducer {
				key := newTestKey(string(rune('A'+producerID)) + "-" + string(rune('0'+j%10)))
				q.Add(key)
			}
		}(i)
	}

	// Consumers
	for range numConsumers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				key, shutdown := q.Get()
				if shutdown {
					return
				}

				processed <- key

				q.Done(key)
			}
		}()
	}

	// Wait for producers to finish
	go func() {
		wg.Wait()
	}()

	// Give time for processing
	time.Sleep(100 * time.Millisecond)

	// Shutdown and wait
	q.ShutDown()
	wg.Wait()
	close(processed)

	// Verify we got some items
	count := 0
	for range processed {
		count++
	}

	if count == 0 {
		t.Error("No items were processed")
	}
}

func TestQueue_RateLimited(t *testing.T) {
	t.Parallel()

	baseDelay := 10 * time.Millisecond
	maxDelay := 100 * time.Millisecond
	rateLimiter := workqueue.NewExponentialBackoffRateLimiter(baseDelay, maxDelay)

	q := workqueue.NewWithRateLimiter(rateLimiter)
	defer q.ShutDown()

	key := newTestKey("test-1")

	start := time.Now()

	q.AddRateLimited(key)

	got, _ := q.Get()
	elapsed := time.Since(start)

	if got != key {
		t.Errorf("Get() = %v, want %v", got, key)
	}

	if elapsed < baseDelay {
		t.Errorf("First AddRateLimited: Get() returned after %v, want at least %v", elapsed, baseDelay)
	}

	q.Done(key)
	q.Forget(key)
}

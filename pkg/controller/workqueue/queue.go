package workqueue

import (
	"sync"
	"time"

	"github.com/sivchari/keel/pkg/core"
)

// Queue implements core.WorkQueue with deduplication, delayed addition, and rate limiting.
type Queue struct {
	mu sync.Mutex

	// queue holds items ready to be processed
	queue []core.ResourceKey

	// dirty tracks items that need processing
	dirty map[core.ResourceKey]struct{}

	// processing tracks items currently being processed
	processing map[core.ResourceKey]struct{}

	// cond signals when items are available or shutdown
	cond *sync.Cond

	// shuttingDown indicates the queue is shutting down
	shuttingDown bool

	// rateLimiter for AddRateLimited
	rateLimiter core.RateLimiter

	// delayedMu protects delayed items
	delayedMu sync.Mutex
	// delayed tracks items waiting to be added
	delayed map[core.ResourceKey]*time.Timer
}

// New creates a new Queue with default rate limiter.
func New() *Queue {
	return NewWithRateLimiter(NewExponentialBackoffRateLimiter(
		5*time.Millisecond,
		1000*time.Second,
	))
}

// NewWithRateLimiter creates a new Queue with the given rate limiter.
func NewWithRateLimiter(rateLimiter core.RateLimiter) *Queue {
	q := &Queue{
		dirty:       make(map[core.ResourceKey]struct{}),
		processing:  make(map[core.ResourceKey]struct{}),
		rateLimiter: rateLimiter,
		delayed:     make(map[core.ResourceKey]*time.Timer),
	}
	q.cond = sync.NewCond(&q.mu)

	return q
}

// Add adds an item to the queue.
func (q *Queue) Add(key core.ResourceKey) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.shuttingDown {
		return
	}

	// If already marked dirty, nothing to do
	if _, exists := q.dirty[key]; exists {
		return
	}

	q.dirty[key] = struct{}{}

	// If currently being processed, don't add to queue yet
	if _, processing := q.processing[key]; processing {
		return
	}

	q.queue = append(q.queue, key)
	q.cond.Signal()
}

// AddAfter adds an item to the queue after the specified duration.
func (q *Queue) AddAfter(key core.ResourceKey, duration time.Duration) {
	if duration <= 0 {
		q.Add(key)

		return
	}

	q.delayedMu.Lock()
	defer q.delayedMu.Unlock()

	// Cancel existing timer for this key
	if timer, exists := q.delayed[key]; exists {
		timer.Stop()
	}

	q.delayed[key] = time.AfterFunc(duration, func() {
		q.delayedMu.Lock()
		delete(q.delayed, key)
		q.delayedMu.Unlock()
		q.Add(key)
	})
}

// AddRateLimited adds an item to the queue with rate limiting.
func (q *Queue) AddRateLimited(key core.ResourceKey) {
	delay := q.rateLimiter.When(key)
	q.AddAfter(key, delay)
}

// Get blocks until an item is available or the queue is shut down.
func (q *Queue) Get() (key core.ResourceKey, shutdown bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.queue) == 0 && !q.shuttingDown {
		q.cond.Wait()
	}

	if q.shuttingDown {
		return core.ResourceKey{}, true
	}

	key = q.queue[0]
	q.queue = q.queue[1:]

	q.processing[key] = struct{}{}
	delete(q.dirty, key)

	return key, false
}

// Done marks an item as processed.
func (q *Queue) Done(key core.ResourceKey) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.processing, key)

	// If item was re-added while being processed, add it back to queue
	if _, dirty := q.dirty[key]; dirty {
		q.queue = append(q.queue, key)
		q.cond.Signal()
	}
}

// Forget resets the rate limiter for an item.
func (q *Queue) Forget(key core.ResourceKey) {
	q.rateLimiter.Forget(key)
}

// Len returns the number of items in the queue.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.queue)
}

// ShutDown shuts down the queue.
func (q *Queue) ShutDown() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.shuttingDown {
		return
	}

	q.shuttingDown = true
	q.queue = nil
	q.cond.Broadcast()

	// Cancel all delayed timers
	q.delayedMu.Lock()

	for _, timer := range q.delayed {
		timer.Stop()
	}

	q.delayed = make(map[core.ResourceKey]*time.Timer)
	q.delayedMu.Unlock()
}

// ShuttingDown returns true if the queue is shutting down.
func (q *Queue) ShuttingDown() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.shuttingDown
}

// Ensure Queue implements core.WorkQueue.
var _ core.WorkQueue = (*Queue)(nil)

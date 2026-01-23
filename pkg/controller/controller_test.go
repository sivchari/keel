package controller_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sivchari/keel/pkg/controller"
	"github.com/sivchari/keel/pkg/core"
)

func newTestKey(name string) core.ResourceKey {
	return core.ResourceKey{
		APIVersion: "test/v1",
		Kind:       "Test",
		Name:       name,
	}
}

// mockSource is a test Source that sends events on demand.
type mockSource struct {
	started chan struct{}
	events  chan core.ResourceKey
}

func newMockSource() *mockSource {
	return &mockSource{
		started: make(chan struct{}),
		events:  make(chan core.ResourceKey, 100),
	}
}

func (s *mockSource) Start(ctx context.Context, handler core.EventHandler, _ core.WorkQueue) error {
	close(s.started)

	for {
		select {
		case <-ctx.Done():
			return nil
		case key := <-s.events:
			handler.OnCreate(ctx, key)
		}
	}
}

func (s *mockSource) SendEvent(key core.ResourceKey) {
	s.events <- key
}

func (s *mockSource) WaitStarted() {
	<-s.started
}

func TestController_New(t *testing.T) {
	t.Parallel()

	reconciler := core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		return core.Result{}, nil
	})

	ctrl := controller.New("test-controller", reconciler, controller.Options{})

	if ctrl.Name() != "test-controller" {
		t.Errorf("Name() = %q, want %q", ctrl.Name(), "test-controller")
	}

	if ctrl.Queue() == nil {
		t.Error("Queue() returned nil")
	}
}

func TestController_Reconcile(t *testing.T) {
	t.Parallel()

	reconciled := make(chan core.ResourceKey, 10)

	reconciler := core.ReconcilerFunc(func(_ context.Context, req core.Request) (core.Result, error) {
		reconciled <- req.Key

		return core.Result{}, nil
	})

	ctrl := controller.New("test-controller", reconciler, controller.Options{})

	source := newMockSource()
	ctrl.Watch(source)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start controller in background.
	done := make(chan error, 1)

	go func() {
		done <- ctrl.Start(ctx)
	}()

	// Wait for source to start.
	source.WaitStarted()

	// Send event.
	key := newTestKey("test-1")
	source.SendEvent(key)

	// Wait for reconciliation.
	select {
	case got := <-reconciled:
		if got != key {
			t.Errorf("reconciled key = %v, want %v", got, key)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for reconciliation")
	}

	// Shutdown.
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for controller to stop")
	}
}

func TestController_ReconcileRequeue(t *testing.T) {
	t.Parallel()

	var count atomic.Int32

	reconciler := core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		count.Add(1)

		// Requeue the first time.
		if count.Load() == 1 {
			return core.Result{Requeue: true}, nil
		}

		return core.Result{}, nil
	})

	ctrl := controller.New("test-controller", reconciler, controller.Options{})

	source := newMockSource()
	ctrl.Watch(source)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- ctrl.Start(ctx)
	}()

	source.WaitStarted()

	key := newTestKey("test-1")
	source.SendEvent(key)

	// Wait for multiple reconciliations.
	time.Sleep(100 * time.Millisecond)

	if got := count.Load(); got < 2 {
		t.Errorf("reconcile count = %d, want >= 2 (should have requeued)", got)
	}

	cancel()

	<-done
}

func TestController_ReconcileRequeueAfter(t *testing.T) {
	t.Parallel()

	tracker := newTimeTracker()
	reconciler := core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		if tracker.record() == 1 {
			return core.Result{RequeueAfter: 50 * time.Millisecond}, nil
		}

		return core.Result{}, nil
	})

	ctrl := controller.New("test-controller", reconciler, controller.Options{})
	source := newMockSource()
	ctrl.Watch(source)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = ctrl.Start(ctx) }()

	source.WaitStarted()
	source.SendEvent(newTestKey("test-1"))
	time.Sleep(200 * time.Millisecond)

	if got := tracker.count(); got < 2 {
		t.Errorf("reconcile count = %d, want >= 2", got)
	}

	if delay := tracker.delayBetween(0, 1); delay < 50*time.Millisecond {
		t.Errorf("delay = %v, want >= 50ms", delay)
	}

	cancel()
}

type timeTracker struct {
	mu    sync.Mutex
	times []time.Time
}

func newTimeTracker() *timeTracker {
	return &timeTracker{}
}

func (t *timeTracker) record() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.times = append(t.times, time.Now())

	return len(t.times)
}

func (t *timeTracker) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	return len(t.times)
}

func (t *timeTracker) delayBetween(i, j int) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	if j >= len(t.times) || i >= len(t.times) {
		return 0
	}

	return t.times[j].Sub(t.times[i])
}

func TestController_ConcurrentReconcile(t *testing.T) {
	t.Parallel()

	var active atomic.Int32

	var maxActive atomic.Int32

	reconciler := core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		current := active.Add(1)

		for {
			old := maxActive.Load()
			if current <= old {
				break
			}

			if maxActive.CompareAndSwap(old, current) {
				break
			}
		}

		time.Sleep(50 * time.Millisecond)

		active.Add(-1)

		return core.Result{}, nil
	})

	ctrl := controller.New("test-controller", reconciler, controller.Options{
		MaxConcurrentReconciles: 3,
	})

	source := newMockSource()
	ctrl.Watch(source)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- ctrl.Start(ctx)
	}()

	source.WaitStarted()

	// Send multiple events.
	for i := range 10 {
		source.SendEvent(newTestKey("test-" + string(rune('0'+i))))
	}

	// Wait for processing.
	time.Sleep(300 * time.Millisecond)

	if got := maxActive.Load(); got > 3 {
		t.Errorf("max concurrent reconciles = %d, want <= 3", got)
	}

	cancel()

	<-done
}

func TestController_StartTwice(t *testing.T) {
	t.Parallel()

	reconciler := core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		return core.Result{}, nil
	})

	ctrl := controller.New("test-controller", reconciler, controller.Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start first time in background.
	go func() {
		_ = ctrl.Start(ctx)
	}()

	// Give it time to start.
	time.Sleep(50 * time.Millisecond)

	// Try to start again.
	err := ctrl.Start(ctx)
	if err == nil {
		t.Error("expected error when starting controller twice")
	}

	cancel()
}

func TestController_WatchAfterStart(t *testing.T) {
	t.Parallel()

	reconciler := core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		return core.Result{}, nil
	})

	ctrl := controller.New("test-controller", reconciler, controller.Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start first.
	go func() {
		_ = ctrl.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Watch after start should not panic but should be a no-op.
	source := newMockSource()

	ctrl.Watch(source) // Should log warning but not fail

	cancel()
}

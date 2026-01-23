// Package controller provides a reconciliation controller implementation.
package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sivchari/keel/pkg/controller/workqueue"
	"github.com/sivchari/keel/pkg/core"
)

// Controller runs a reconciliation loop for resources.
type Controller struct {
	// name identifies this controller.
	name string

	// reconciler performs the actual reconciliation.
	reconciler core.Reconciler

	// queue holds items to be processed.
	queue core.WorkQueue

	// sources produce events that trigger reconciliation.
	sources []core.Source

	// maxConcurrentReconciles limits concurrent reconciliations.
	maxConcurrentReconciles int

	// logger for this controller.
	logger *slog.Logger

	// started indicates if the controller has been started.
	started bool

	// mu protects started.
	mu sync.Mutex
}

// Options configures a Controller.
type Options struct {
	// MaxConcurrentReconciles limits concurrent reconciliations.
	// Defaults to 1.
	MaxConcurrentReconciles int

	// RateLimiter for the work queue.
	// If nil, uses the default exponential backoff rate limiter.
	RateLimiter core.RateLimiter

	// Logger for this controller.
	// If nil, uses slog.Default().
	Logger *slog.Logger
}

// New creates a new Controller with the given name and reconciler.
func New(name string, reconciler core.Reconciler, opts Options) *Controller {
	if opts.MaxConcurrentReconciles <= 0 {
		opts.MaxConcurrentReconciles = 1
	}

	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	var queue core.WorkQueue
	if opts.RateLimiter != nil {
		queue = workqueue.NewWithRateLimiter(opts.RateLimiter)
	} else {
		queue = workqueue.New()
	}

	return &Controller{
		name:                    name,
		reconciler:              reconciler,
		queue:                   queue,
		maxConcurrentReconciles: opts.MaxConcurrentReconciles,
		logger:                  opts.Logger.With("controller", name),
	}
}

// Watch adds a source to the controller.
func (c *Controller) Watch(source core.Source) *Controller {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		c.logger.Warn("cannot add source after controller started")

		return c
	}

	c.sources = append(c.sources, source)

	return c
}

// Start runs the controller until the context is cancelled.
func (c *Controller) Start(ctx context.Context) error {
	c.mu.Lock()

	if c.started {
		c.mu.Unlock()

		return errors.New("controller already started")
	}

	c.started = true
	c.mu.Unlock()

	c.logger.Info("starting controller", "workers", c.maxConcurrentReconciles)

	// Start sources.
	var wg sync.WaitGroup

	errCh := make(chan error, len(c.sources))

	for i, source := range c.sources {
		wg.Add(1)

		go func(idx int, src core.Source) {
			defer wg.Done()

			handler := core.EnqueueEventHandler(c.queue)

			if err := src.Start(ctx, handler, c.queue); err != nil {
				errCh <- fmt.Errorf("source %d failed: %w", idx, err)
			}
		}(i, source)
	}

	// Start workers.
	for range c.maxConcurrentReconciles {
		wg.Add(1)

		go func() {
			defer wg.Done()

			c.runWorker(ctx)
		}()
	}

	// Wait for context cancellation.
	<-ctx.Done()

	c.logger.Info("shutting down controller")
	c.queue.ShutDown()

	// Wait for all goroutines to finish.
	wg.Wait()

	// Check for source errors.
	close(errCh)

	var errs []error

	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// runWorker processes items from the queue until shutdown.
func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

// processNextItem processes a single item from the queue.
func (c *Controller) processNextItem(ctx context.Context) bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	defer c.queue.Done(key)

	result, err := c.reconcile(ctx, key)
	if err != nil {
		c.logger.Error("reconciliation failed",
			"key", key.String(),
			"error", err,
		)

		c.queue.AddRateLimited(key)

		return true
	}

	// Successful reconciliation, reset rate limiter.
	c.queue.Forget(key)

	// Handle requeue.
	if result.RequeueAfter > 0 {
		c.queue.AddAfter(key, result.RequeueAfter)
	} else if result.Requeue {
		c.queue.Add(key)
	}

	return true
}

// reconcile calls the reconciler with appropriate logging.
func (c *Controller) reconcile(ctx context.Context, key core.ResourceKey) (core.Result, error) {
	start := time.Now()

	c.logger.Debug("reconciling", "key", key.String())

	req := core.Request{Key: key}

	result, err := c.reconciler.Reconcile(ctx, req)

	duration := time.Since(start)

	if err != nil {
		c.logger.Error("reconciliation error",
			"key", key.String(),
			"duration", duration,
			"error", err,
		)

		return core.Result{}, err
	}

	c.logger.Debug("reconciliation complete",
		"key", key.String(),
		"duration", duration,
		"requeue", result.Requeue,
		"requeueAfter", result.RequeueAfter,
	)

	return result, nil
}

// Name returns the controller name.
func (c *Controller) Name() string {
	return c.name
}

// Queue returns the controller's work queue.
func (c *Controller) Queue() core.WorkQueue {
	return c.queue
}

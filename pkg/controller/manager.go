package controller

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// Manager manages multiple controllers and coordinates their lifecycle.
type Manager struct {
	// controllers managed by this manager.
	controllers []*Controller

	// logger for this manager.
	logger *slog.Logger

	// started indicates if the manager has been started.
	started bool

	// mu protects started and controllers.
	mu sync.Mutex
}

// ManagerOptions configures a Manager.
type ManagerOptions struct {
	// Logger for this manager.
	// If nil, uses slog.Default().
	Logger *slog.Logger
}

// NewManager creates a new Manager.
func NewManager(opts ManagerOptions) *Manager {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &Manager{
		logger: opts.Logger.With("component", "manager"),
	}
}

// Add registers a controller with the manager.
func (m *Manager) Add(controller *Controller) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return errors.New("cannot add controller after manager started")
	}

	m.controllers = append(m.controllers, controller)

	return nil
}

// Start runs all controllers until the context is cancelled.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()

	if m.started {
		m.mu.Unlock()

		return errors.New("manager already started")
	}

	m.started = true
	m.mu.Unlock()

	m.logger.Info("starting manager", "controllers", len(m.controllers))

	var wg sync.WaitGroup

	errCh := make(chan error, len(m.controllers))

	// Start all controllers.
	for _, ctrl := range m.controllers {
		wg.Add(1)

		go func(c *Controller) {
			defer wg.Done()

			if err := c.Start(ctx); err != nil {
				errCh <- err
			}
		}(ctrl)
	}

	// Wait for all controllers to finish.
	wg.Wait()

	close(errCh)

	// Collect errors.
	var errs []error

	for err := range errCh {
		errs = append(errs, err)
	}

	m.logger.Info("manager stopped")

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// Controllers returns the list of managed controllers.
func (m *Manager) Controllers() []*Controller {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*Controller, len(m.controllers))
	copy(result, m.controllers)

	return result
}

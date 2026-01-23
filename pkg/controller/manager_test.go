package controller_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sivchari/keel/pkg/controller"
	"github.com/sivchari/keel/pkg/core"
)

func TestManager_New(t *testing.T) {
	t.Parallel()

	mgr := controller.NewManager(controller.ManagerOptions{})

	if mgr == nil {
		t.Fatal("NewManager() returned nil")
	}

	controllers := mgr.Controllers()

	if len(controllers) != 0 {
		t.Errorf("Controllers() = %d, want 0", len(controllers))
	}
}

func TestManager_Add(t *testing.T) {
	t.Parallel()

	mgr := controller.NewManager(controller.ManagerOptions{})

	reconciler := core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		return core.Result{}, nil
	})

	ctrl1 := controller.New("ctrl-1", reconciler, controller.Options{})
	ctrl2 := controller.New("ctrl-2", reconciler, controller.Options{})

	if err := mgr.Add(ctrl1); err != nil {
		t.Errorf("Add(ctrl1) failed: %v", err)
	}

	if err := mgr.Add(ctrl2); err != nil {
		t.Errorf("Add(ctrl2) failed: %v", err)
	}

	controllers := mgr.Controllers()

	if len(controllers) != 2 {
		t.Errorf("Controllers() = %d, want 2", len(controllers))
	}
}

func TestManager_Start(t *testing.T) {
	t.Parallel()

	mgr := controller.NewManager(controller.ManagerOptions{})

	var flag1, flag2 atomic.Bool

	ctrl1 := controller.New("ctrl-1", flagReconciler(&flag1), controller.Options{})
	ctrl2 := controller.New("ctrl-2", flagReconciler(&flag2), controller.Options{})

	source1, source2 := newMockSource(), newMockSource()
	ctrl1.Watch(source1)
	ctrl2.Watch(source2)

	if err := mgr.Add(ctrl1); err != nil {
		t.Fatalf("Add(ctrl1) failed: %v", err)
	}

	if err := mgr.Add(ctrl2); err != nil {
		t.Fatalf("Add(ctrl2) failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = mgr.Start(ctx) }()

	source1.WaitStarted()
	source2.WaitStarted()
	source1.SendEvent(newTestKey("test-1"))
	source2.SendEvent(newTestKey("test-2"))
	time.Sleep(100 * time.Millisecond)

	if !flag1.Load() {
		t.Error("ctrl1 was not reconciled")
	}

	if !flag2.Load() {
		t.Error("ctrl2 was not reconciled")
	}

	cancel()
}

func flagReconciler(flag *atomic.Bool) core.Reconciler {
	return core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		flag.Store(true)

		return core.Result{}, nil
	})
}

func TestManager_StartTwice(t *testing.T) {
	t.Parallel()

	mgr := controller.NewManager(controller.ManagerOptions{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start first time in background.
	go func() {
		_ = mgr.Start(ctx)
	}()

	// Give it time to start.
	time.Sleep(50 * time.Millisecond)

	// Try to start again.
	err := mgr.Start(ctx)
	if err == nil {
		t.Error("expected error when starting manager twice")
	}

	cancel()
}

func TestManager_AddAfterStart(t *testing.T) {
	t.Parallel()

	mgr := controller.NewManager(controller.ManagerOptions{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start first.
	go func() {
		_ = mgr.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	reconciler := core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		return core.Result{}, nil
	})

	ctrl := controller.New("ctrl", reconciler, controller.Options{})

	// Add after start should fail.
	err := mgr.Add(ctrl)
	if err == nil {
		t.Error("expected error when adding controller after start")
	}

	cancel()
}

func TestManager_ControllersReturnsDefensiveCopy(t *testing.T) {
	t.Parallel()

	mgr := controller.NewManager(controller.ManagerOptions{})

	reconciler := core.ReconcilerFunc(func(_ context.Context, _ core.Request) (core.Result, error) {
		return core.Result{}, nil
	})

	ctrl := controller.New("ctrl", reconciler, controller.Options{})

	if err := mgr.Add(ctrl); err != nil {
		t.Fatalf("Add() failed: %v", err)
	}

	// Get controllers.
	controllers := mgr.Controllers()

	// Modify the returned slice.
	controllers[0] = nil

	// Original should be unaffected.
	original := mgr.Controllers()

	if original[0] == nil {
		t.Error("Controllers() did not return a defensive copy")
	}
}

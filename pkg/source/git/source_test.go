package git_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/sivchari/keel/pkg/core"
	"github.com/sivchari/keel/pkg/source/git"
)

type eventRecorder struct {
	mu      sync.Mutex
	creates []core.ResourceKey
	updates []core.ResourceKey
	deletes []core.ResourceKey
}

func (r *eventRecorder) OnCreate(_ context.Context, key core.ResourceKey) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.creates = append(r.creates, key)
}

func (r *eventRecorder) OnUpdate(_ context.Context, key core.ResourceKey) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.updates = append(r.updates, key)
}

func (r *eventRecorder) OnDelete(_ context.Context, key core.ResourceKey) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.deletes = append(r.deletes, key)
}

func (r *eventRecorder) getCreates() []core.ResourceKey {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]core.ResourceKey{}, r.creates...)
}

func (r *eventRecorder) getUpdates() []core.ResourceKey {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]core.ResourceKey{}, r.updates...)
}

func (r *eventRecorder) getDeletes() []core.ResourceKey {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]core.ResourceKey{}, r.deletes...)
}

func setupTestRepo(t *testing.T) (string, *gogit.Repository) {
	t.Helper()

	dir := t.TempDir()

	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	return dir, repo
}

func commitFile(t *testing.T, repo *gogit.Repository, dir, filename, content string) {
	t.Helper()

	fullPath := filepath.Join(dir, filename)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	if _, err := w.Add(filename); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	_, err = w.Commit("add "+filename, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
}

func deleteFile(t *testing.T, repo *gogit.Repository, dir, filename string) {
	t.Helper()

	fullPath := filepath.Join(dir, filename)

	if err := os.Remove(fullPath); err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	if _, err := w.Remove(filename); err != nil {
		t.Fatalf("failed to stage removal: %v", err)
	}

	_, err = w.Commit("delete "+filename, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    git.Options
		wantErr bool
	}{
		{
			name:    "empty path",
			opts:    git.Options{},
			wantErr: true,
		},
		{
			name: "valid path",
			opts: git.Options{
				Path: "/tmp/test",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := git.New(tt.opts)

			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSource_InitialSync(t *testing.T) {
	t.Parallel()

	dir, repo := setupTestRepo(t)

	resource := `apiVersion: aws.keel.dev/v1alpha1
kind: S3Bucket
metadata:
  name: test-bucket
spec:
  bucketName: my-bucket
`
	commitFile(t, repo, dir, "resources/bucket.yaml", resource)

	src, err := git.New(git.Options{
		Path:         dir,
		PollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	recorder := &eventRecorder{}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = src.Start(ctx, recorder, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	creates := recorder.getCreates()
	if len(creates) != 1 {
		t.Errorf("expected 1 create event, got %d", len(creates))
	}

	if len(creates) > 0 {
		if creates[0].Name != "test-bucket" {
			t.Errorf("expected resource name 'test-bucket', got %q", creates[0].Name)
		}

		if creates[0].Kind != "S3Bucket" {
			t.Errorf("expected kind 'S3Bucket', got %q", creates[0].Kind)
		}
	}
}

func TestSource_DetectNewFile(t *testing.T) {
	t.Parallel()

	dir, repo := setupTestRepo(t)

	initialResource := `apiVersion: aws.keel.dev/v1alpha1
kind: S3Bucket
metadata:
  name: bucket-1
spec:
  bucketName: bucket-1
`
	commitFile(t, repo, dir, "bucket1.yaml", initialResource)

	src, err := git.New(git.Options{
		Path:         dir,
		PollInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	recorder := &eventRecorder{}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = src.Start(ctx, recorder, nil)
	}()

	time.Sleep(100 * time.Millisecond)

	newResource := `apiVersion: aws.keel.dev/v1alpha1
kind: S3Bucket
metadata:
  name: bucket-2
spec:
  bucketName: bucket-2
`
	commitFile(t, repo, dir, "bucket2.yaml", newResource)

	time.Sleep(150 * time.Millisecond)
	cancel()

	creates := recorder.getCreates()
	if len(creates) != 2 {
		t.Errorf("expected 2 create events, got %d", len(creates))
	}
}

func TestSource_DetectDeletedFile(t *testing.T) {
	t.Parallel()

	dir, repo := setupTestRepo(t)

	resource := `apiVersion: aws.keel.dev/v1alpha1
kind: S3Bucket
metadata:
  name: test-bucket
spec:
  bucketName: my-bucket
`
	commitFile(t, repo, dir, "bucket.yaml", resource)

	src, err := git.New(git.Options{
		Path:         dir,
		PollInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	recorder := &eventRecorder{}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = src.Start(ctx, recorder, nil)
	}()

	time.Sleep(100 * time.Millisecond)

	deleteFile(t, repo, dir, "bucket.yaml")

	time.Sleep(150 * time.Millisecond)
	cancel()

	deletes := recorder.getDeletes()
	if len(deletes) != 1 {
		t.Errorf("expected 1 delete event, got %d", len(deletes))
	}
}

func TestSource_DetectModifiedFile(t *testing.T) {
	t.Parallel()

	dir, repo := setupTestRepo(t)

	resource := `apiVersion: aws.keel.dev/v1alpha1
kind: S3Bucket
metadata:
  name: test-bucket
  generation: 1
spec:
  bucketName: my-bucket
`
	commitFile(t, repo, dir, "bucket.yaml", resource)

	src, err := git.New(git.Options{
		Path:         dir,
		PollInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	recorder := &eventRecorder{}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = src.Start(ctx, recorder, nil)
	}()

	time.Sleep(100 * time.Millisecond)

	updatedResource := `apiVersion: aws.keel.dev/v1alpha1
kind: S3Bucket
metadata:
  name: test-bucket
  generation: 2
spec:
  bucketName: my-bucket-updated
`
	commitFile(t, repo, dir, "bucket.yaml", updatedResource)

	time.Sleep(150 * time.Millisecond)
	cancel()

	updates := recorder.getUpdates()
	if len(updates) != 1 {
		t.Errorf("expected 1 update event, got %d", len(updates))
	}
}

func TestSource_JSONFile(t *testing.T) {
	t.Parallel()

	dir, repo := setupTestRepo(t)

	resource := `{
  "apiVersion": "aws.keel.dev/v1alpha1",
  "kind": "S3Bucket",
  "metadata": {
    "name": "json-bucket"
  },
  "spec": {
    "bucketName": "my-json-bucket"
  }
}`
	commitFile(t, repo, dir, "bucket.json", resource)

	src, err := git.New(git.Options{
		Path:         dir,
		PollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	recorder := &eventRecorder{}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = src.Start(ctx, recorder, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	creates := recorder.getCreates()
	if len(creates) != 1 {
		t.Errorf("expected 1 create event, got %d", len(creates))
	}

	if len(creates) > 0 && creates[0].Name != "json-bucket" {
		t.Errorf("expected resource name 'json-bucket', got %q", creates[0].Name)
	}
}

func TestSource_MultiDocumentYAML(t *testing.T) {
	t.Parallel()

	dir, repo := setupTestRepo(t)

	resources := `apiVersion: aws.keel.dev/v1alpha1
kind: S3Bucket
metadata:
  name: bucket-1
spec:
  bucketName: bucket-1
---
apiVersion: aws.keel.dev/v1alpha1
kind: S3Bucket
metadata:
  name: bucket-2
spec:
  bucketName: bucket-2
`
	commitFile(t, repo, dir, "buckets.yaml", resources)

	src, err := git.New(git.Options{
		Path:         dir,
		PollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	recorder := &eventRecorder{}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = src.Start(ctx, recorder, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	creates := recorder.getCreates()
	if len(creates) != 2 {
		t.Errorf("expected 2 create events, got %d", len(creates))
	}
}

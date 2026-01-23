// Package git provides a Git repository source for the keel reconciliation engine.
package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"

	"github.com/sivchari/keel/pkg/core"
)

// Source watches a Git repository for resource changes.
type Source struct {
	opts   Options
	repo   *git.Repository
	logger *slog.Logger

	mu       sync.Mutex
	lastHash plumbing.Hash
	cache    map[string]*core.Resource
}

// Get returns the resource with the given key, or nil if not found.
func (s *Source) Get(key core.ResourceKey) *core.Resource {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.cache[key.String()]
}

// New creates a new Git source with the given options.
func New(opts Options) (*Source, error) {
	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	opts.setDefaults()

	return &Source{
		opts:   opts,
		logger: opts.Logger.With("source", "git", "path", opts.Path),
		cache:  make(map[string]*core.Resource),
	}, nil
}

// Start begins watching the Git repository for changes.
func (s *Source) Start(ctx context.Context, handler core.EventHandler, _ core.WorkQueue) error {
	if err := s.openRepository(); err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	if err := s.initialSync(ctx, handler); err != nil {
		return fmt.Errorf("initial sync failed: %w", err)
	}

	s.logger.Info("starting git source", "interval", s.opts.PollInterval)

	ticker := time.NewTicker(s.opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("stopping git source")

			return nil
		case <-ticker.C:
			if err := s.poll(ctx, handler); err != nil {
				s.logger.Error("poll failed", "error", err)
			}
		}
	}
}

func (s *Source) openRepository() error {
	repo, err := git.PlainOpen(s.opts.Path)
	if err != nil {
		return fmt.Errorf("failed to open repository at %s: %w", s.opts.Path, err)
	}

	s.repo = repo

	return nil
}

func (s *Source) initialSync(ctx context.Context, handler core.EventHandler) error {
	head, err := s.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	s.mu.Lock()
	s.lastHash = head.Hash()
	s.mu.Unlock()

	resources, err := s.scanResources()
	if err != nil {
		return fmt.Errorf("failed to scan resources: %w", err)
	}

	s.mu.Lock()

	for key, res := range resources {
		s.cache[key] = res

		handler.OnCreate(ctx, res.Key)
	}

	s.mu.Unlock()

	s.logger.Info("initial sync complete", "resources", len(resources))

	return nil
}

func (s *Source) poll(ctx context.Context, handler core.EventHandler) error {
	if s.opts.Pull {
		if err := s.pullChanges(); err != nil {
			s.logger.Warn("pull failed, continuing with local state", "error", err)
		}
	}

	head, err := s.repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	s.mu.Lock()
	lastHash := s.lastHash
	s.mu.Unlock()

	if head.Hash() == lastHash {
		return nil
	}

	s.logger.Debug("detected changes", "old", lastHash.String()[:7], "new", head.Hash().String()[:7])

	changes, err := s.detectChanges(lastHash, head.Hash())
	if err != nil {
		return fmt.Errorf("failed to detect changes: %w", err)
	}

	s.processChanges(ctx, handler, changes)

	s.mu.Lock()
	s.lastHash = head.Hash()
	s.mu.Unlock()

	return nil
}

func (s *Source) pullChanges() error {
	w, err := s.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = w.Pull(&git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: plumbing.NewBranchReferenceName(s.opts.Branch),
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("pull failed: %w", err)
	}

	return nil
}

func (s *Source) detectChanges(oldHash, newHash plumbing.Hash) (object.Changes, error) {
	oldCommit, err := s.repo.CommitObject(oldHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get old commit: %w", err)
	}

	newCommit, err := s.repo.CommitObject(newHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get new commit: %w", err)
	}

	oldTree, err := oldCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get old tree: %w", err)
	}

	newTree, err := newCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get new tree: %w", err)
	}

	changes, err := oldTree.Diff(newTree)
	if err != nil {
		return nil, fmt.Errorf("failed to diff trees: %w", err)
	}

	return changes, nil
}

func (s *Source) processChanges(ctx context.Context, handler core.EventHandler, changes object.Changes) {
	for _, change := range changes {
		if !s.isResourceFile(change.To.Name) && !s.isResourceFile(change.From.Name) {
			continue
		}

		s.processChange(ctx, handler, change)
	}
}

func (s *Source) processChange(ctx context.Context, handler core.EventHandler, change *object.Change) {
	action, err := change.Action()
	if err != nil {
		s.logger.Error("failed to get change action", "error", err)

		return
	}

	switch action {
	case merkletrie.Insert:
		s.handleInsert(ctx, handler, change.To.Name)
	case merkletrie.Delete:
		s.handleDelete(ctx, handler, change.From.Name)
	case merkletrie.Modify:
		s.handleModify(ctx, handler, change.To.Name)
	}
}

func (s *Source) handleInsert(ctx context.Context, handler core.EventHandler, path string) {
	resources, err := s.parseFile(path)
	if err != nil {
		s.logger.Error("failed to parse file", "path", path, "error", err)

		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, res := range resources {
		key := res.Key.String()
		s.cache[key] = res

		handler.OnCreate(ctx, res.Key)
		s.logger.Debug("resource created", "key", key)
	}
}

func (s *Source) handleDelete(ctx context.Context, handler core.EventHandler, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, res := range s.cache {
		if res.SourcePath == path {
			delete(s.cache, key)

			handler.OnDelete(ctx, res.Key)
			s.logger.Debug("resource deleted", "key", key)
		}
	}
}

func (s *Source) handleModify(ctx context.Context, handler core.EventHandler, path string) {
	newResources, err := s.parseFile(path)
	if err != nil {
		s.logger.Error("failed to parse file", "path", path, "error", err)

		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newKeys := make(map[string]bool)

	for _, res := range newResources {
		newKeys[res.Key.String()] = true
	}

	for key, res := range s.cache {
		if res.SourcePath == path && !newKeys[key] {
			delete(s.cache, key)

			handler.OnDelete(ctx, res.Key)
			s.logger.Debug("resource deleted", "key", key)
		}
	}

	for _, res := range newResources {
		key := res.Key.String()

		if existing, ok := s.cache[key]; ok {
			s.cache[key] = res

			if !resourceEqual(existing, res) {
				handler.OnUpdate(ctx, res.Key)
				s.logger.Debug("resource updated", "key", key)
			}
		} else {
			s.cache[key] = res

			handler.OnCreate(ctx, res.Key)
			s.logger.Debug("resource created", "key", key)
		}
	}
}

func (s *Source) scanResources() (map[string]*core.Resource, error) {
	w, err := s.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	resources := make(map[string]*core.Resource)

	root := w.Filesystem.Root()

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("failed to get relative path: %w", relErr)
		}

		if !s.isResourceFile(relPath) {
			return nil
		}

		parsed, parseErr := s.parseFile(relPath)
		if parseErr != nil {
			s.logger.Warn("failed to parse file", "path", relPath, "error", parseErr)

			return nil
		}

		for _, res := range parsed {
			resources[res.Key.String()] = res
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return resources, nil
}

func (s *Source) isResourceFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))

	return ext == ".yaml" || ext == ".yml" || ext == ".json"
}

func resourceEqual(a, b *core.Resource) bool {
	if a.Key != b.Key {
		return false
	}

	if a.Generation != b.Generation {
		return false
	}

	return true
}

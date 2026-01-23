package git

import (
	"errors"
	"log/slog"
	"time"
)

// Options configures a Git source.
type Options struct {
	// Path is the local path to the Git repository.
	Path string

	// Branch is the branch to watch.
	// Defaults to "main".
	Branch string

	// PollInterval is how often to check for changes.
	// Defaults to 30 seconds.
	PollInterval time.Duration

	// Pull enables automatic pulling from remote.
	// Defaults to false.
	Pull bool

	// Logger for this source.
	// Defaults to slog.Default().
	Logger *slog.Logger
}

func (o *Options) validate() error {
	if o.Path == "" {
		return errors.New("path is required")
	}

	return nil
}

func (o *Options) setDefaults() {
	if o.Branch == "" {
		o.Branch = "main"
	}

	if o.PollInterval <= 0 {
		o.PollInterval = 30 * time.Second
	}

	if o.Logger == nil {
		o.Logger = slog.Default()
	}
}

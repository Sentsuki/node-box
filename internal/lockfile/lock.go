// Package lockfile provides a single-writer lock over a directory.
//
// The lock is held by the operating system for as long as the process holds an
// open handle, so it is released automatically when the process exits for any
// reason, crashes included. That is the whole reason for using an OS lock
// rather than a pid file: a pid file left behind by a SIGKILL wedges the next
// start until someone deletes it by hand, which is exactly the situation an
// unattended daemon must not be in.
package lockfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ErrLocked reports that another process holds the lock. It is returned
// instead of a platform-specific errno so callers can test for it.
var ErrLocked = errors.New("lock is held by another process")

// Lock is an acquired lock. Release it when the work it guards is done.
type Lock struct {
	path   string
	handle io.Closer
}

// Acquire takes the lock at path, creating the file and its parent directory if
// needed. It never waits: if another process holds the lock it returns
// ErrLocked immediately, because every caller here would rather report the
// conflict than queue up behind an update of unknown length.
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	handle, err := acquire(path)
	if err != nil {
		if errors.Is(err, ErrLocked) {
			return nil, err
		}
		return nil, fmt.Errorf("acquire lock %s: %w", path, err)
	}
	return &Lock{path: path, handle: handle}, nil
}

// Release gives up the lock. The lock file itself is left in place: removing it
// would race with another process that has already opened it, and an empty file
// costs nothing.
//
// Release is safe to call on a nil Lock, so a caller that may not have taken a
// lock can defer it unconditionally.
func (l *Lock) Release() error {
	if l == nil || l.handle == nil {
		return nil
	}
	err := l.handle.Close()
	l.handle = nil
	return err
}

// Path returns the lock file's location, for error messages.
func (l *Lock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

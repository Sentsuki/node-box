//go:build unix

package lockfile

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// acquire takes a BSD advisory lock on the file.
//
// flock is tied to the open file description rather than the process, and the
// kernel drops it when the descriptor is closed, so the lock cannot outlive the
// process that took it.
func acquire(path string) (io.Closer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return f, nil
}

//go:build windows

package lockfile

import (
	"io"
	"os"
	"syscall"
)

// errSharingViolation is what CreateFile reports when another handle already
// holds the file without sharing. The syscall package does not name it.
const errSharingViolation = syscall.Errno(32)

// acquire opens the file for writing with no sharing, which Windows itself
// enforces as an exclusive claim.
//
// Unlike a pid file this is released by the kernel when the handle closes, so
// it behaves the same way the flock implementation does on Unix.
func acquire(path string) (io.Closer, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	handle, err := syscall.CreateFile(
		name,
		syscall.GENERIC_WRITE,
		0, // no sharing: a second open fails while this handle is alive
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		if err == errSharingViolation || err == syscall.ERROR_ACCESS_DENIED {
			return nil, ErrLocked
		}
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

// Package output turns generated configuration into files on disk.
//
// Writing is atomic and content-addressed: a file whose hash matches the
// previous run is left untouched, and a file that does change is replaced in a
// single rename so a reader never observes a partial write.
package output

import (
	"crypto/sha256"
	"encoding/hex"
)

// Perm is the mode of generated configuration files. They can contain node
// credentials, so they are not world readable.
const Perm = 0o600

// File is one generated configuration file held in memory.
type File struct {
	// Name is the configs[] entry this came from, used in logs and errors.
	Name string
	// Path is the absolute destination.
	Path string
	// Content is the complete file body, including the trailing newline.
	Content []byte
}

// Hash returns the hex-encoded SHA-256 of the content.
func (f File) Hash() string {
	sum := sha256.Sum256(f.Content)
	return hex.EncodeToString(sum[:])
}

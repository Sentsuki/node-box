// Package fsx holds the filesystem primitives shared across node-box.
//
// It has no dependencies of its own, which is the point: snapshot storage,
// generated output and path validation all need the same atomic write and the
// same containment check, and none of them should have to import each other to
// get them.
package fsx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteAtomic replaces path with data in a single rename.
//
// The temporary file is created in the destination's own directory: rename(2)
// only works within one filesystem, and keeping the temp file next to the target
// means that always holds. The leading dot and the .tmp suffix keep the
// half-written file from being picked up by anything scanning for *.json.
func WriteAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)

	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmp := f.Name()

	// Any failure past this point leaves the destination untouched; clean up
	// the temp file. After a successful rename the name is gone and Remove is
	// a harmless no-op.
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()

	if _, err = f.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err = f.Chmod(perm); err != nil {
		return fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmp, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmp, path, err)
	}
	return nil
}

// Within reports whether p is dir itself or lies inside it.
//
// Both paths are compared as the host spells them, so this is the check to use
// for absolute destinations on disk. Repository-relative paths from a config
// file are always slash-separated and are validated separately.
func Within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

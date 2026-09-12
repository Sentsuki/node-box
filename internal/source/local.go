package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Local reads the configuration from a directory on this machine. It exists
// for development and for running without network access; it behaves exactly
// like the GitHub source, including snapshotting and rollback.
type Local struct {
	dir string
}

// NewLocal creates a local source.
func NewLocal(dir string) *Local { return &Local{dir: dir} }

// Describe names the source.
func (l *Local) Describe() string { return "local:" + l.dir }

// Resolve returns a content hash of the directory, so an edit produces a new
// ref exactly the way a commit does.
func (l *Local) Resolve(ctx context.Context) (string, error) {
	h := sha256.New()

	err := filepath.WalkDir(l.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(l.dir, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00", filepath.ToSlash(rel))

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", l.dir, err)
	}

	// 20 bytes is plenty to distinguish local edits and keeps directory names
	// the same length as a git short-ish sha.
	return hex.EncodeToString(h.Sum(nil))[:40], nil
}

// Materialize copies the directory into destDir.
//
// The ref is checked rather than ignored. A local snapshot can only ever be
// taken of what the directory holds right now, so a request for any other ref
// cannot be served — and silently storing current content under the requested
// name would make `update --ref` apply something that is not that ref at all.
// Refusing also catches the narrower case of the directory changing between
// being resolved and being copied.
func (l *Local) Materialize(ctx context.Context, ref, destDir string) error {
	current, err := l.Resolve(ctx)
	if err != nil {
		return err
	}
	if ref != current {
		return fmt.Errorf(
			"%s currently hashes to %s, not %s; a local source can only snapshot its present contents",
			l.dir, current, ref)
	}

	return filepath.WalkDir(l.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		rel, err := filepath.Rel(l.dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(destDir, rel), 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, filepath.Join(destDir, rel))
	})
}

// skipDir reports whether a directory should be excluded from a snapshot.
func skipDir(name string) bool {
	return name == ".git" || strings.HasPrefix(name, ".incoming-")
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

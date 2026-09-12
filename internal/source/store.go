// Package source obtains the configuration repository and keeps immutable
// snapshots of it on disk.
//
// A snapshot directory is written once and never modified. Everything node-box
// generates is derived from one, so a snapshot that produced a working set of
// files can always be re-applied.
package source

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"node-box/internal/logx"
	"node-box/internal/output"
)

// Pointer names kept alongside the snapshot directories.
const (
	// PointerCurrent names the snapshot that produced the files currently on
	// disk. It advances only after a write has succeeded, never at fetch time:
	// a pointer that could name a snapshot which failed to build would make the
	// unreachable-source fallback restore something known to be broken, and
	// would make the change poll believe a failed revision was already done.
	PointerCurrent = "current"
	// PointerPrevious names the snapshot that produced the outputs before the
	// current ones, which is what a rollback goes back to.
	//
	// It is deliberately not "the last snapshot that built successfully": that
	// is always the one currently applied, so rolling back to it would be a
	// no-op for exactly the case rollback exists for — a change that builds
	// fine but turns out to be wrong.
	PointerPrevious = "previous"
)

// DefaultKeep is how many snapshots are retained by GC.
const DefaultKeep = 10

// Store manages the snapshots directory.
//
// current and previous are plain text files holding a ref, not symlinks:
// a pointer file is replaced atomically by the same rename used everywhere
// else, and works identically on every platform.
type Store struct {
	dir string
}

// NewStore prepares the snapshots directory.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create snapshots directory %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the directory a ref is stored in.
func (s *Store) Dir(ref string) string { return filepath.Join(s.dir, ref) }

// Has reports whether a snapshot is already on disk.
func (s *Store) Has(ref string) bool {
	if !validRef(ref) {
		return false
	}
	info, err := os.Stat(s.Dir(ref))
	return err == nil && info.IsDir()
}

// Pointer reads a pointer file, reporting whether it is set and usable.
// A pointer to a snapshot that no longer exists is treated as unset.
func (s *Store) Pointer(name string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return "", false
	}
	ref := strings.TrimSpace(string(data))
	if ref == "" || !s.Has(ref) {
		return "", false
	}
	return ref, true
}

// SetPointer points name at ref.
func (s *Store) SetPointer(name, ref string) error {
	if !s.Has(ref) {
		return fmt.Errorf("cannot point %s at unknown snapshot %q", name, ref)
	}
	path := filepath.Join(s.dir, name)
	return output.WriteAtomic(path, []byte(ref+"\n"), 0o600)
}

// Begin creates a scratch directory to build a snapshot in.
//
// Extraction happens in the scratch directory and only becomes visible under
// its real name via Commit, so an interrupted fetch can never be mistaken for
// a complete snapshot.
func (s *Store) Begin(ref string) (string, error) {
	if !validRef(ref) {
		return "", fmt.Errorf("invalid ref %q", ref)
	}
	tmp, err := os.MkdirTemp(s.dir, ".incoming-"+ref+"-*")
	if err != nil {
		return "", fmt.Errorf("create scratch directory: %w", err)
	}
	return tmp, nil
}

// Commit moves a scratch directory into place as ref.
//
// If the snapshot already exists the scratch copy is discarded: snapshots are
// immutable, so an existing one is by definition identical.
func (s *Store) Commit(tmp, ref string) error {
	target := s.Dir(ref)
	if s.Has(ref) {
		os.RemoveAll(tmp)
		return nil
	}
	if err := os.Rename(tmp, target); err != nil {
		os.RemoveAll(tmp)
		return fmt.Errorf("commit snapshot %s: %w", ref, err)
	}
	return nil
}

// Abort discards a scratch directory.
func (s *Store) Abort(tmp string) {
	if tmp != "" {
		os.RemoveAll(tmp)
	}
}

// List returns the refs currently stored, oldest first.
func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read snapshots directory: %w", err)
	}

	type entry struct {
		ref    string
		modSec int64
	}
	var refs []entry
	for _, e := range entries {
		if !e.IsDir() || !validRef(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		refs = append(refs, entry{e.Name(), info.ModTime().UnixNano()})
	}
	slices.SortFunc(refs, func(a, b entry) int {
		if a.modSec != b.modSec {
			return int(a.modSec - b.modSec)
		}
		return strings.Compare(a.ref, b.ref)
	})

	out := make([]string, len(refs))
	for i, e := range refs {
		out[i] = e.ref
	}
	return out, nil
}

// GC removes all but the newest keep snapshots. Whatever current and previous
// point at is always retained, however old it is.
func (s *Store) GC(keep int) error {
	if keep <= 0 {
		keep = DefaultKeep
	}
	refs, err := s.List()
	if err != nil {
		return err
	}
	if len(refs) <= keep {
		return nil
	}

	pinned := map[string]bool{}
	for _, name := range []string{PointerCurrent, PointerPrevious} {
		if ref, ok := s.Pointer(name); ok {
			pinned[ref] = true
		}
	}

	// refs is oldest first, so the tail is what we keep.
	for _, ref := range refs[:len(refs)-keep] {
		if pinned[ref] {
			continue
		}
		if err := os.RemoveAll(s.Dir(ref)); err != nil {
			logx.Warnf("could not remove old snapshot %s: %v", ref, err)
			continue
		}
		logx.Debugf("removed old snapshot %s", ref)
	}
	return nil
}

// CleanScratch removes leftover scratch directories from interrupted fetches.
func (s *Store) CleanScratch() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), ".incoming-") {
			path := filepath.Join(s.dir, e.Name())
			if err := os.RemoveAll(path); err == nil {
				logx.Debugf("removed leftover scratch directory %s", e.Name())
			}
		}
	}
}

// validRef restricts refs to characters safe in a path component, so a ref
// can never escape the snapshots directory.
func validRef(ref string) bool {
	if ref == "" || len(ref) > 128 {
		return false
	}
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

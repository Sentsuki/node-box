package output

import (
	"fmt"
	"os"
	"path/filepath"

	"node-box/internal/logx"
	"node-box/internal/model"
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

// Result reports what a Write call did.
type Result struct {
	Written []string // paths whose content changed
	Skipped []string // paths whose hash matched the previous run
}

// Write persists files, skipping any whose hash matches prev.
//
// It returns the new hash map for every file regardless of whether it was
// written, so callers can persist it as the next run's baseline. Each file is
// independently atomic: if one write fails the earlier ones stay committed and
// the error names the file that failed.
func Write(files []File, prev map[string]string, force bool) (Result, map[string]string, error) {
	var res Result
	hashes := make(map[string]string, len(files))

	for _, f := range files {
		h := f.Hash()
		hashes[f.Path] = h

		if !force && prev[f.Path] == h && exists(f.Path) {
			res.Skipped = append(res.Skipped, f.Path)
			logx.Debugf("output %s unchanged, skipping write", f.Path)
			continue
		}
		if err := WriteAtomic(f.Path, f.Content, Perm); err != nil {
			return res, hashes, fmt.Errorf("write output %q: %w", f.Name, err)
		}
		res.Written = append(res.Written, f.Path)
		logx.Debugf("wrote %s (%d bytes)", f.Path, len(f.Content))
	}
	return res, hashes, nil
}

// EnsureDirs prepares destination directories before any build work happens, so
// a bad path fails at startup rather than after a full fetch-and-assemble cycle.
//
// Directories under outputDir are created on demand. A destination outside it is
// required to exist already: silently creating directories from a mistyped
// absolute path is far harder to diagnose than an error.
func EnsureDirs(outs []model.ResolvedOutput, outputDir string) error {
	for _, o := range outs {
		dir := filepath.Dir(o.Path)

		if within(dir, outputDir) {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return fmt.Errorf("config %q: create output directory %s: %w", o.Config.Name, dir, err)
			}
		} else {
			info, err := os.Stat(dir)
			if err != nil {
				return fmt.Errorf("config %q: output directory %s must already exist: %w", o.Config.Name, dir, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("config %q: output path %s is not a directory", o.Config.Name, dir)
			}
		}

		if err := checkWritable(dir); err != nil {
			return fmt.Errorf("config %q: output directory %s is not writable: %w", o.Config.Name, dir, err)
		}
	}
	return nil
}

// checkWritable verifies the process can create files in dir by actually
// creating one. Inspecting permission bits would not account for ownership,
// ACLs or a read-only mount.
func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".node-box-writable-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// within reports whether p is dir itself or lies inside it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel != ".." && !hasDotDotPrefix(rel)
}

func hasDotDotPrefix(rel string) bool {
	const dotdot = ".." + string(filepath.Separator)
	return len(rel) >= len(dotdot) && rel[:len(dotdot)] == dotdot
}

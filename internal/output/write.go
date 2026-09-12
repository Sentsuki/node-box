package output

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"node-box/internal/fsx"
	"node-box/internal/logx"
)

// Result reports what a Write call did.
type Result struct {
	Written []string // paths whose content changed
	Skipped []string // paths whose hash matched the previous run
}

// Write persists files, skipping any whose content already matches on disk.
//
// The comparison is against the file itself rather than a remembered hash, so
// the decision cannot drift from reality: a deleted or hand-edited output is
// restored on the next run, and a lost state file costs nothing.
//
// It returns the hash of every file regardless of whether it was written, so
// callers can record what they are managing. Each file is independently
// atomic: if one write fails the earlier ones stay committed and the error
// names the file that failed.
func Write(files []File, force bool) (Result, map[string]string, error) {
	var res Result
	hashes := make(map[string]string, len(files))

	for _, f := range files {
		hashes[f.Path] = f.Hash()

		if !force && matchesDisk(f) {
			res.Skipped = append(res.Skipped, f.Path)
			logx.Debugf("output %s unchanged, skipping write", f.Path)
			continue
		}
		if err := fsx.WriteAtomic(f.Path, f.Content, Perm); err != nil {
			return res, hashes, fmt.Errorf("write output %q: %w", f.Name, err)
		}
		res.Written = append(res.Written, f.Path)
		logx.Debugf("wrote %s (%d bytes)", f.Path, len(f.Content))
	}
	return res, hashes, nil
}

// matchesDisk reports whether the destination already holds exactly this
// content. Any read error counts as a mismatch so the file gets written.
func matchesDisk(f File) bool {
	existing, err := os.ReadFile(f.Path)
	return err == nil && bytes.Equal(existing, f.Content)
}

// CheckDirs reports whether the destinations look usable, without creating or
// writing anything.
//
// It exists so the pipeline can reject a mistyped path before it spends a full
// fetch-and-assemble cycle, while leaving `build` and `validate` genuinely free
// of side effects. A destination outside outputDir is required to exist
// already: silently creating directories from a mistyped absolute path is far
// harder to diagnose than an error. One inside it is accepted whether or not it
// exists yet, because EnsureDirs will create it.
func CheckDirs(outs []Target, outputDir string) error {
	for _, o := range outs {
		dir := filepath.Dir(o.Path)
		if fsx.Within(dir, outputDir) {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("config %q: output directory %s must already exist: %w", o.Config.Name, dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("config %q: output path %s is not a directory", o.Config.Name, dir)
		}
	}
	return nil
}

// EnsureDirs creates the destination directories under outputDir and verifies
// that every destination can actually be written to.
//
// Unlike CheckDirs this changes the filesystem, so it belongs to the writing
// step rather than to planning.
func EnsureDirs(outs []Target, outputDir string) error {
	if err := CheckDirs(outs, outputDir); err != nil {
		return err
	}
	for _, o := range outs {
		dir := filepath.Dir(o.Path)

		if fsx.Within(dir, outputDir) {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return fmt.Errorf("config %q: create output directory %s: %w", o.Config.Name, dir, err)
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

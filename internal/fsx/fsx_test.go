package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// leftovers reports temp files the writer failed to clean up.
func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var found []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			found = append(found, e.Name())
		}
	}
	return found
}

func TestWriteAtomic_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := WriteAtomic(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	if got := read(t, path); got != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
	if extra := leftovers(t, dir); len(extra) > 0 {
		t.Errorf("temp files left behind: %v", extra)
	}

	// Windows does not model POSIX permission bits.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("mode = %o, want %o", got, 0o600)
		}
	}
}

func TestWriteAtomic_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte("old content that is longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	if got := read(t, path); got != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
	if extra := leftovers(t, dir); len(extra) > 0 {
		t.Errorf("temp files left behind: %v", extra)
	}
}

func TestWriteAtomic_FailureLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	// A directory where the target name already exists as a directory makes
	// the final rename fail, exercising the cleanup path.
	path := filepath.Join(dir, "target")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := WriteAtomic(path, []byte("data"), 0o600); err == nil {
		t.Fatal("want an error when the destination is a directory")
	}
	if extra := leftovers(t, dir); len(extra) > 0 {
		t.Errorf("temp files left behind after failure: %v", extra)
	}
}

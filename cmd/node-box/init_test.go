package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCmdInit_WritesTheSkeletonInAStableOrder(t *testing.T) {
	dir := t.TempDir()

	out := captureStdout(t, func() {
		if err := cmdInit(nil, &env{}, []string{dir}); err != nil {
			t.Fatalf("init: %v", err)
		}
	})

	// Every declared file lands on disk. The mode is deliberately not asserted:
	// Windows synthesises it, so the check would mean nothing there.
	for name := range starterFiles {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Errorf("%s was not created: %v", name, err)
		}
	}

	// The listing is what the operator reads to check what init did, so it must
	// not reorder itself between runs.
	var listed []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if name, ok := strings.CutPrefix(line, "create "); ok {
			listed = append(listed, name)
		}
	}
	if len(listed) != len(starterFiles) {
		t.Fatalf("listed %d created files, want %d", len(listed), len(starterFiles))
	}
	if !slices.IsSorted(listed) {
		t.Errorf("created files were listed out of order: %v", listed)
	}
}

func TestCmdInit_SkipsExistingFilesUnlessForced(t *testing.T) {
	dir := t.TempDir()
	if err := cmdInit(nil, &env{}, []string{dir}); err != nil {
		t.Fatal(err)
	}

	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte("{ edited }"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without --force an existing file is left alone: init must never eat edits.
	out := captureStdout(t, func() {
		if err := cmdInit(nil, &env{}, []string{dir}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "skip   config.json") {
		t.Errorf("want config.json reported as skipped, got:\n%s", out)
	}
	if data, _ := os.ReadFile(config); string(data) != "{ edited }" {
		t.Error("an existing file was overwritten without --force")
	}

	if err := cmdInit(nil, &env{}, []string{"--force", dir}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(config); string(data) == "{ edited }" {
		t.Error("--force should have rewritten the file")
	}
}

// captureStdout collects what fn prints.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = saved }()

	fn()
	w.Close()

	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	r.Close()
	return b.String()
}

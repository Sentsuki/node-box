package output

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"node-box/internal/model"
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

	if err := WriteAtomic(path, []byte("hello"), Perm); err != nil {
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
		if got := info.Mode().Perm(); got != Perm {
			t.Errorf("mode = %o, want %o", got, Perm)
		}
	}
}

func TestWriteAtomic_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte("old content that is longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("new"), Perm); err != nil {
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

	if err := WriteAtomic(path, []byte("data"), Perm); err == nil {
		t.Fatal("want an error when the destination is a directory")
	}
	if extra := leftovers(t, dir); len(extra) > 0 {
		t.Errorf("temp files left behind after failure: %v", extra)
	}
}

func TestWrite_SkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	f := File{Name: "main", Path: filepath.Join(dir, "main.json"), Content: []byte("{}\n")}

	// First run: nothing known, so it writes.
	res, hashes, err := Write([]File{f}, nil, false)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(res.Written) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("first run: written=%v skipped=%v", res.Written, res.Skipped)
	}

	// Second run with the same content: skipped.
	res, _, err = Write([]File{f}, hashes, false)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(res.Written) != 0 || len(res.Skipped) != 1 {
		t.Fatalf("second run: written=%v skipped=%v", res.Written, res.Skipped)
	}

	// Changed content: written again.
	f.Content = []byte(`{"log":{}}` + "\n")
	res, _, err = Write([]File{f}, hashes, false)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(res.Written) != 1 {
		t.Fatalf("changed content should be written, got %v", res)
	}
}

func TestWrite_ForceIgnoresHash(t *testing.T) {
	dir := t.TempDir()
	f := File{Name: "main", Path: filepath.Join(dir, "main.json"), Content: []byte("{}\n")}

	_, hashes, err := Write([]File{f}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := Write([]File{f}, hashes, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 {
		t.Errorf("force should rewrite, got %v", res)
	}
}

func TestWrite_RewritesWhenFileVanished(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.json")
	f := File{Name: "main", Path: path, Content: []byte("{}\n")}

	_, hashes, err := Write([]File{f}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	// A matching hash must not be trusted when the file is gone: someone
	// deleted it, and the state file should not keep us from restoring it.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	res, _, err := Write([]File{f}, hashes, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 {
		t.Errorf("deleted file should be rewritten, got %v", res)
	}
}

func TestEnsureDirs(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "out")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}

	t.Run("creates nested dirs under output dir", func(t *testing.T) {
		outs := []model.ResolvedOutput{{
			Config: model.ConfigFile{Name: "nested"},
			Path:   filepath.Join(outDir, "a", "b", "main.json"),
		}}
		if err := EnsureDirs(outs, outDir); err != nil {
			t.Fatalf("EnsureDirs: %v", err)
		}
		if _, err := os.Stat(filepath.Join(outDir, "a", "b")); err != nil {
			t.Errorf("directory was not created: %v", err)
		}
	})

	t.Run("refuses to create dirs outside output dir", func(t *testing.T) {
		outs := []model.ResolvedOutput{{
			Config: model.ConfigFile{Name: "elsewhere"},
			Path:   filepath.Join(root, "does-not-exist", "config.json"),
		}}
		err := EnsureDirs(outs, outDir)
		if err == nil {
			t.Fatal("want an error for a missing directory outside the output dir")
		}
		if !strings.Contains(err.Error(), "elsewhere") {
			t.Errorf("error should name the config: %v", err)
		}
	})

	t.Run("accepts existing dirs outside output dir", func(t *testing.T) {
		other := filepath.Join(root, "etc")
		if err := os.MkdirAll(other, 0o700); err != nil {
			t.Fatal(err)
		}
		outs := []model.ResolvedOutput{{
			Config: model.ConfigFile{Name: "etc"},
			Path:   filepath.Join(other, "config.json"),
		}}
		if err := EnsureDirs(outs, outDir); err != nil {
			t.Fatalf("EnsureDirs: %v", err)
		}
	})
}

func TestState_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Missing file is not an error: a fresh install starts from empty.
	s, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState on missing file: %v", err)
	}
	if s.Ref != "" || len(s.Outputs) != 0 {
		t.Errorf("fresh state should be empty, got %+v", s)
	}

	s.RecordSuccess("abc123", map[string]string{"/out/main.json": "deadbeef"})
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.Ref != "abc123" {
		t.Errorf("ref = %q, want abc123", loaded.Ref)
	}
	if loaded.Outputs["/out/main.json"] != "deadbeef" {
		t.Errorf("outputs = %v", loaded.Outputs)
	}
}

func TestState_CorruptFileStartsOver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A corrupt cache must not wedge the process; the cost is one extra rewrite.
	s, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Ref != "" || s.Outputs == nil {
		t.Errorf("want a usable empty state, got %+v", s)
	}
}

package fileops_test

import (
	"os"
	"path/filepath"
	"testing"

	"node-box/internal/fileops"
)

// ---------------------------------------------------------------------------
// Scanner – single file mode
// ---------------------------------------------------------------------------

func TestScanner_SingleFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	s := fileops.NewScanner(path, true)
	files, err := s.ScanConfigFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0] != path {
		t.Errorf("expected [%s], got %v", path, files)
	}
}

func TestScanner_SingleFile_NotFound(t *testing.T) {
	s := fileops.NewScanner("/nonexistent/path/config.json", true)
	_, err := s.ScanConfigFiles()
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestScanner_SingleFile_WrongExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	s := fileops.NewScanner(path, true)
	_, err := s.ScanConfigFiles()
	if err == nil {
		t.Error("expected error for non-.json file in single file mode")
	}
}

// ---------------------------------------------------------------------------
// Scanner – directory mode
// ---------------------------------------------------------------------------

func TestScanner_Directory_FindsJSONFiles(t *testing.T) {
	dir := t.TempDir()

	// Create JSON files
	files := []string{"a.json", "b.json", "c.json"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(`{}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Create non-JSON file (should be ignored)
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0644); err != nil {
		t.Fatal(err)
	}

	s := fileops.NewScanner(dir, false)
	found, err := s.ScanConfigFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 3 {
		t.Errorf("expected 3 JSON files, got %d: %v", len(found), found)
	}
}

func TestScanner_Directory_Recursive(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "root.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	s := fileops.NewScanner(dir, false)
	found, err := s.ScanConfigFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 2 {
		t.Errorf("expected 2 JSON files (recursive), got %d: %v", len(found), found)
	}
}

func TestScanner_Directory_Empty(t *testing.T) {
	dir := t.TempDir()
	s := fileops.NewScanner(dir, false)
	found, err := s.ScanConfigFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("expected 0 files in empty directory, got %d", len(found))
	}
}

func TestScanner_Directory_CaseInsensitiveExtension(t *testing.T) {
	dir := t.TempDir()
	// Create files with different case extensions
	for _, name := range []string{"a.json", "b.JSON", "c.Json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	s := fileops.NewScanner(dir, false)
	found, err := s.ScanConfigFiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 3 {
		t.Errorf("expected 3 files (case-insensitive), got %d: %v", len(found), found)
	}
}

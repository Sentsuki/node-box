package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tarEntry is one file or directory to place in a test archive.
type tarEntry struct {
	name     string
	body     string
	typeflag byte
	linkname string
}

// makeTarGz builds a gzipped tar from entries.
func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for _, e := range entries {
		flag := e.typeflag
		if flag == 0 {
			flag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     0o644,
			Size:     int64(len(e.body)),
			Typeflag: flag,
			Linkname: e.linkname,
		}
		if flag != tar.TypeReg {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if flag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractTarGz_StripsRootDirectory(t *testing.T) {
	// GitHub wraps the repository in one directory named after the commit.
	archive := makeTarGz(t, []tarEntry{
		{name: "you-repo-abc123/", typeflag: tar.TypeDir},
		{name: "you-repo-abc123/config.json", body: `{"a":1}`},
		{name: "you-repo-abc123/modules/", typeflag: tar.TypeDir},
		{name: "you-repo-abc123/modules/dns.json", body: `{"dns":{}}`},
	})

	dir := t.TempDir()
	if err := extractTarGz(bytes.NewReader(archive), dir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("config.json missing: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("config.json = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "modules", "dns.json")); err != nil {
		t.Errorf("modules/dns.json missing: %v", err)
	}
}

func TestExtractTarGz_RejectsPathTraversal(t *testing.T) {
	// An archive is untrusted input; a "../" entry must never write outside
	// the destination.
	archive := makeTarGz(t, []tarEntry{
		{name: "you-repo-abc123/", typeflag: tar.TypeDir},
		{name: "you-repo-abc123/../../escaped.json", body: "pwned"},
	})

	dir := t.TempDir()
	err := extractTarGz(bytes.NewReader(archive), dir)
	if err == nil {
		t.Fatal("want an error for an entry that escapes the destination")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("error should say the entry escapes: %v", err)
	}
}

func TestExtractTarGz_SkipsSymlinks(t *testing.T) {
	// A symlink pointing outside the tree is the classic extraction exploit:
	// a later entry writes "through" it.
	archive := makeTarGz(t, []tarEntry{
		{name: "you-repo-abc123/", typeflag: tar.TypeDir},
		{name: "you-repo-abc123/link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
		{name: "you-repo-abc123/config.json", body: "{}"},
	})

	dir := t.TempDir()
	if err := extractTarGz(bytes.NewReader(archive), dir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "link")); err == nil {
		t.Error("the symlink should have been skipped")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("regular files should still be extracted: %v", err)
	}
}

func TestExtractTarGz_RejectsTooManyFiles(t *testing.T) {
	entries := []tarEntry{{name: "r/", typeflag: tar.TypeDir}}
	for i := range MaxArchiveFiles + 1 {
		entries = append(entries, tarEntry{name: "r/f" + itoa(i) + ".json", body: "{}"})
	}
	err := extractTarGz(bytes.NewReader(makeTarGz(t, entries)), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("want a file-count error, got %v", err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestStore_PointersAndGC(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// A pointer that was never set reads as unset.
	if _, ok := store.Pointer(PointerCurrent); ok {
		t.Error("current should start unset")
	}

	mk := func(ref string) {
		t.Helper()
		tmp, err := store.Begin(ref)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "config.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := store.Commit(tmp, ref); err != nil {
			t.Fatal(err)
		}
	}

	mk("aaa")
	if err := store.SetPointer(PointerCurrent, "aaa"); err != nil {
		t.Fatal(err)
	}
	if ref, ok := store.Pointer(PointerCurrent); !ok || ref != "aaa" {
		t.Errorf("current = %q %v, want aaa true", ref, ok)
	}

	// A pointer to a snapshot that has been removed reads as unset rather
	// than handing back a ref that cannot be opened.
	os.RemoveAll(store.Dir("aaa"))
	if _, ok := store.Pointer(PointerCurrent); ok {
		t.Error("a pointer to a missing snapshot should read as unset")
	}
}

func TestStore_GCKeepsPinnedSnapshots(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	refs := []string{"aaa", "bbb", "ccc", "ddd", "eee"}
	for _, ref := range refs {
		tmp, err := store.Begin(ref)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(tmp, "config.json"), []byte("{}"), 0o600)
		if err := store.Commit(tmp, ref); err != nil {
			t.Fatal(err)
		}
	}

	// Pin the oldest, then keep only 2.
	if err := store.SetPointer(PointerLastGood, "aaa"); err != nil {
		t.Fatal(err)
	}
	if err := store.GC(2); err != nil {
		t.Fatal(err)
	}

	if !store.Has("aaa") {
		t.Error("the pinned last-good snapshot must survive GC however old it is")
	}
	if !store.Has("eee") {
		t.Error("the newest snapshot should be kept")
	}
}

func TestStore_RejectsUnsafeRefs(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// A ref becomes a path component, so anything that could escape the
	// snapshots directory has to be refused.
	for _, ref := range []string{"../evil", "a/b", "", ".", "with space", "semi;colon"} {
		if _, err := store.Begin(ref); err == nil {
			t.Errorf("ref %q should be rejected", ref)
		}
		if store.Has(ref) {
			t.Errorf("ref %q should never report as present", ref)
		}
	}
}

func TestStore_CleanScratch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// An interrupted fetch leaves a scratch directory behind.
	tmp, err := store.Begin("abc")
	if err != nil {
		t.Fatal(err)
	}
	store.CleanScratch()
	if _, err := os.Stat(tmp); err == nil {
		t.Error("leftover scratch directory should have been removed")
	}
}

func TestLocal_ResolveChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"a":1}`), 0o600)

	l := NewLocal(dir)
	ctx := context.Background()

	first, err := l.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again, err := l.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Error("an unchanged directory should resolve to the same ref")
	}
	if !validRef(first) {
		t.Errorf("ref %q is not usable as a directory name", first)
	}

	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"a":2}`), 0o600)
	changed, err := l.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Error("an edit should produce a new ref")
	}
}

func TestLocal_MaterializeSkipsGit(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "config.json"), []byte("{}"), 0o600)
	os.MkdirAll(filepath.Join(src, ".git"), 0o700)
	os.WriteFile(filepath.Join(src, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o600)

	dst := t.TempDir()
	if err := NewLocal(src).Materialize(context.Background(), "ref", dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "config.json")); err != nil {
		t.Errorf("config.json should be copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err == nil {
		t.Error(".git should not be copied into a snapshot")
	}
}

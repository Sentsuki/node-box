package lockfile

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquire_CreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "update.lock")

	lock, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer lock.Release()

	if lock.Path() != path {
		t.Errorf("Path() = %q, want %q", lock.Path(), path)
	}
}

func TestAcquire_SecondHolderIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.lock")

	first, err := Acquire(path)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	// Both implementations are per-handle rather than per-process, so a second
	// attempt is refused even from here.
	if _, err := Acquire(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Acquire = %v, want ErrLocked", err)
	}
}

func TestAcquire_AfterReleaseSucceeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.lock")

	first, err := Acquire(path)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	second, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
}

func TestRelease_IsSafeOnNilAndTwice(t *testing.T) {
	var none *Lock
	if err := none.Release(); err != nil {
		t.Errorf("Release on nil lock: %v", err)
	}
	if none.Path() != "" {
		t.Error("Path on nil lock should be empty")
	}

	lock, err := Acquire(filepath.Join(t.TempDir(), "update.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("first Release: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Errorf("second Release should be a no-op, got %v", err)
	}
}

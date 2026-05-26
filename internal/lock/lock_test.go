package lock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireRejectsLiveProcessLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.lock")
	if err := os.WriteFile(path, []byte("pid=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Acquire(path)
	if err == nil {
		_ = got.Release()
		t.Fatal("expected live process lock to be rejected")
	}
	if !strings.Contains(err.Error(), "lock held by pid 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAcquireReplacesStalePIDLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.lock")
	if err := os.WriteFile(path, []byte("pid=99999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Release()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "pid="
	if !strings.Contains(string(data), want) || strings.Contains(string(data), "99999999") {
		t.Fatalf("expected stale lock to be replaced, got %q", data)
	}
}

func TestAcquireReplacesLegacyLockFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.lock")
	if err := os.WriteFile(path, []byte("locked\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Release()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "pid=") {
		t.Fatalf("expected legacy lock to be replaced, got %q", data)
	}
}

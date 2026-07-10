package lock

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWithRunsAndReleases(t *testing.T) {
	dir := t.TempDir()
	ran := false

	err := With(dir, func() error { ran = true; return nil })
	if err != nil || !ran {
		t.Fatalf("With: err=%v ran=%v", err, ran)
	}
	if _, err := os.Lstat(filepath.Join(dir, "lock")); !os.IsNotExist(err) {
		t.Fatal("lock not released")
	}
}

func TestWithBreaksStaleSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A dead holder pid encoded in the lock symlink (mirrors state_lock.bats).
	if err := os.Symlink("999999", filepath.Join(dir, "lock")); err != nil {
		t.Fatal(err)
	}

	ran := false
	err := With(dir, func() error { ran = true; return nil })
	if err != nil || !ran {
		t.Fatalf("stale lock not broken: err=%v ran=%v", err, ran)
	}
}

func TestWithIsMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	inside, maxInside := 0, 0

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = With(dir, func() error {
				mu.Lock()
				inside++
				if inside > maxInside {
					maxInside = inside
				}
				mu.Unlock()
				time.Sleep(5 * time.Millisecond)
				mu.Lock()
				inside--
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()

	if maxInside != 1 {
		t.Fatalf("maxInside = %d, want 1 (lock is not exclusive)", maxInside)
	}
}

func TestWithReleasesOnPanic(t *testing.T) {
	dir := t.TempDir()
	func() {
		defer func() { _ = recover() }()
		_ = With(dir, func() error { panic("boom") })
	}()
	if _, err := os.Lstat(filepath.Join(dir, "lock")); !os.IsNotExist(err) {
		t.Fatal("lock not released after panic")
	}
}

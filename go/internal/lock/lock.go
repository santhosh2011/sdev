// Package lock implements sdev's portable state lock, byte-compatible with
// with_state_lock in bin/_lib.sh so Go and bash contend on the same lock: an
// atomic symlink whose target is the holder pid, broken only when that pid is
// dead, with ramped backoff under a wall-clock deadline.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/santhosh2011/sdev/internal/proc"
)

const (
	// busyDeadline gives up waiting for a live-held lock (wall-clock, matching
	// _STATE_LOCK_BUSY_SECS) so it means the same on a fast or a loaded box.
	busyDeadline = 120 * time.Second
	// staleGrace: a pid-less legacy lock older than this is force-broken.
	staleGrace = 10 * time.Second
)

// With acquires the state lock in stateDir, runs fn, and releases it even if fn
// panics (mirroring the bash EXIT-trap release). A crash or os.Exit mid-hold
// leaks a stale lock that the next acquirer self-heals.
func With(stateDir string, fn func() error) (err error) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(stateDir, "lock")
	if err := acquire(lockPath); err != nil {
		return err
	}
	traceEnter(stateDir)
	defer func() {
		traceExit(stateDir)
		if rerr := release(lockPath); rerr != nil && err == nil {
			err = rerr
		}
	}()
	return fn()
}

func acquire(lockPath string) error {
	self := strconv.Itoa(os.Getpid())
	start := time.Now()
	tries := 0
	for {
		// os.Symlink is atomic and fails if the path exists, so there is no
		// pid-less window for a waiter to misjudge.
		if err := os.Symlink(self, lockPath); err == nil {
			return nil
		}
		breakIfStale(lockPath)
		if time.Since(start) >= busyDeadline {
			return fmt.Errorf("state lock busy: %s (remove it if no sdev is running)", lockPath)
		}
		tries++
		switch {
		case tries < 10:
			time.Sleep(10 * time.Millisecond) // fast path: uncontended release
		case tries < 50:
			time.Sleep(50 * time.Millisecond)
		default:
			time.Sleep(200 * time.Millisecond) // heavy contention: let the holder run
		}
	}
}

func release(lockPath string) error {
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// breakIfStale removes the lock only when its holder is genuinely dead. It claims
// the lock via an atomic rename and then RE-VERIFIES the claimed target is still
// dead before discarding it: a lock re-acquired during handoff (whose prior
// target a waiter read as dead) must not be broken, else two processes end up in
// the critical section. If the claimed lock turns out live, it is restored via a
// non-clobbering symlink create.
func breakIfStale(lockPath string) {
	fi, err := os.Lstat(lockPath)
	if err != nil {
		return // vanished mid-check - retry, never break
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		breakLegacyLock(lockPath, fi)
		return
	}
	target, err := os.Readlink(lockPath)
	if err != nil || !targetDead(target) {
		return // live or unreadable - keep
	}
	// Re-read: a genuine handoff (the prior holder released and a new one
	// acquired) shows a DIFFERENT target on the second read, so back off. Only a
	// target that reads the SAME dead pid twice is a truly abandoned lock. This
	// closes the check-vs-reacquire race that a single stale read opens.
	if again, err := os.Readlink(lockPath); err != nil || again != target || !targetDead(again) {
		return
	}
	// Claim atomically, then discard only if it is still the exact abandoned lock
	// we confirmed; otherwise restore via a non-clobbering symlink create.
	grave := fmt.Sprintf("%s.stale.%d", lockPath, os.Getpid())
	if os.Rename(lockPath, grave) != nil {
		return // lost the race to claim - another waiter is handling it
	}
	claimed, _ := os.Readlink(grave)
	if claimed == target {
		os.Remove(grave) // broke exactly the abandoned lock
		return
	}
	_ = os.Symlink(claimed, lockPath)
	os.Remove(grave)
}

// targetDead reports whether a lock's symlink target (a "<pid>" or "<pid>:...")
// names a dead process. An empty, "0", or unparseable target is treated as live
// so it is never broken (it may be mid-write).
func targetDead(target string) bool {
	pid := target
	if i := strings.IndexByte(target, ':'); i >= 0 {
		pid = target[:i]
	}
	if pid == "" || pid == "0" {
		return false
	}
	n, err := strconv.Atoi(pid)
	if err != nil {
		return false
	}
	return !proc.Alive(n, "")
}

// breakLegacyLock handles a dir/file lock left by a pre-symlink sdev (upgrade
// path): break a dead-pid one, or a pid-less one older than the grace period.
func breakLegacyLock(lockPath string, fi os.FileInfo) {
	if data, err := os.ReadFile(filepath.Join(lockPath, "pid")); err == nil {
		pid := strings.TrimSpace(string(data))
		if pid != "" {
			if n, err := strconv.Atoi(pid); err == nil && proc.Alive(n, "") {
				return
			}
			_ = os.RemoveAll(lockPath)
			return
		}
	}
	if time.Since(fi.ModTime()) >= staleGrace {
		_ = os.RemoveAll(lockPath)
	}
}

// traceEnter/traceExit are a gated concurrency self-check: when SDEV_LOCK_TRACE
// is set, an O_EXCL HOLDER marker atomically detects a second holder inside the
// critical section and appends a DOUBLEHOLD line. Zero cost (one getenv) when
// unset; the lock-interop test enables it to assert mutual exclusion.
func traceEnter(stateDir string) {
	if os.Getenv("SDEV_LOCK_TRACE") == "" {
		return
	}
	holder := filepath.Join(stateDir, "HOLDER")
	id := fmt.Sprintf("go:%d", os.Getpid())
	if f, err := os.OpenFile(holder, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err != nil {
		existing, _ := os.ReadFile(holder)
		appendTrace(fmt.Sprintf("DOUBLEHOLD %s saw %s", id, string(existing)))
	} else {
		fmt.Fprint(f, id)
		f.Close()
	}
}

func traceExit(stateDir string) {
	if os.Getenv("SDEV_LOCK_TRACE") == "" {
		return
	}
	os.Remove(filepath.Join(stateDir, "HOLDER"))
}

func appendTrace(line string) {
	path := os.Getenv("SDEV_LOCK_TRACE")
	if path == "" {
		return
	}
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(f, line)
		f.Close()
	}
}

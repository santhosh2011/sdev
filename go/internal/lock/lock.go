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

	// Ramped backoff: cheap spins first (the common uncontended release), then
	// longer sleeps under contention so the holder's work is not starved. This
	// ramp is load-bearing (see AGENTS.md "Locking").
	backoffFastTries   = 10
	backoffMediumTries = 50
	backoffFast        = 10 * time.Millisecond
	backoffMedium      = 50 * time.Millisecond
	backoffSlow        = 200 * time.Millisecond
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
		case tries < backoffFastTries:
			time.Sleep(backoffFast)
		case tries < backoffMediumTries:
			time.Sleep(backoffMedium)
		default:
			time.Sleep(backoffSlow)
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

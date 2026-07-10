package state

import (
	"fmt"
	"path/filepath"
)

// ReservePoolSlot bumps the monotonic pool sequence and returns the destination
// path for a returned worktree: state/pool/<project>/<repoPath>.<seq>. Mirrors
// _pool_reserve_slot_locked; the caller holds the state lock.
func ReservePoolSlot(home, project, repoPath string) (string, error) {
	var slot string
	err := mutate(home, func(l *Ledger) {
		l.PoolSeq++
		slot = filepath.Join(PoolDir(home), project, fmt.Sprintf("%s.%d", repoPath, l.PoolSeq))
	})
	return slot, err
}

// RecordPool appends a warm-pool entry, stamping ReturnedAt when unset. Mirrors
// _pool_record_locked; the caller holds the state lock.
func RecordPool(home string, e PoolEntry) error {
	if e.ReturnedAt == "" {
		e.ReturnedAt = nowUTC()
	}
	return mutate(home, func(l *Ledger) {
		l.Pool = append(l.Pool, e)
	})
}

// DropPool removes every warm-pool entry whose Path equals path, keeping the
// rest. A path with no matching entry is a no-op. Mirrors _pool_drop_locked in
// bin/_lib.sh; the caller holds the state lock.
func DropPool(home, path string) error {
	return mutate(home, func(l *Ledger) {
		kept := l.Pool[:0]
		for _, e := range l.Pool {
			if e.Path != path {
				kept = append(kept, e)
			}
		}
		l.Pool = kept
	})
}

// TakePool removes and returns the path of the first warm-pool entry whose
// Source matches, or "" if none. Mirrors _pool_take_locked; the caller holds the
// state lock.
func TakePool(home, source string) (string, error) {
	var taken string
	err := mutate(home, func(l *Ledger) {
		for i, e := range l.Pool {
			if e.Source == source {
				taken = e.Path
				l.Pool = append(l.Pool[:i], l.Pool[i+1:]...)
				return
			}
		}
	})
	return taken, err
}

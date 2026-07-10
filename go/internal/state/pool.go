package state

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

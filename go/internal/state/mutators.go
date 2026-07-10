package state

import "path/filepath"

// SetLease records a durable lease on key, creating a bare ledger entry (offset
// adopted from the task's .env) if none exists. The caller holds the lock.
func SetLease(home, key, holder string) error {
	return mutate(home, func(l *Ledger) {
		ensureEntry(home, l, key)
		t := l.Tasks[key]
		t.Lease = true
		t.LeaseHolder = holder
		l.Tasks[key] = t
	})
}

// ProcLock identifies a process-lock: the holder pid and its start-time token.
type ProcLock struct {
	Pid   int
	Token string
}

// SetLock records a process-lock on key, creating a bare entry if needed. The
// caller holds the lock.
func SetLock(home, key string, lock ProcLock) error {
	return mutate(home, func(l *Ledger) {
		ensureEntry(home, l, key)
		t := l.Tasks[key]
		t.Pid = lock.Pid
		t.ProcToken = lock.Token
		l.Tasks[key] = t
	})
}

// ClearReservation drops a task's lease and process-lock while keeping its
// offset. A key with no entry is a no-op. The caller holds the lock.
func ClearReservation(home, key string) error {
	return mutate(home, func(l *Ledger) {
		t, ok := l.Tasks[key]
		if !ok {
			return
		}
		t.Lease = false
		t.LeaseHolder = ""
		t.Pid = 0
		t.ProcToken = ""
		l.Tasks[key] = t
	})
}

// mutate loads the ledger, applies fn, and saves it. The caller holds the lock.
func mutate(home string, fn func(*Ledger)) error {
	if err := Init(home); err != nil {
		return err
	}
	l, err := Load(FilePath(home))
	if err != nil {
		return err
	}
	fn(l)
	return Save(home, l)
}

// ensureEntry creates a bare ledger entry for an existing on-disk task if absent,
// adopting its .env PORT_OFFSET. Mirrors _ensure_task_entry_locked.
func ensureEntry(home string, l *Ledger, key string) {
	if _, ok := l.Tasks[key]; ok {
		return
	}
	off := envOffset(filepath.Join(home, "projects", key, ".env"))
	if off < 0 {
		off = 0
	}
	l.Tasks[key] = Task{Offset: off, CreatedAt: nowUTC()}
}

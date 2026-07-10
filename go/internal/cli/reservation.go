package cli

import (
	"github.com/santhosh2011/sdev/internal/proc"
	"github.com/santhosh2011/sdev/internal/state"
)

// taskLive reports whether a task holds a live lease or a live process-lock - the
// two states that protect it from an unforced destroy or an automatic prune. A
// stale lock (dead pid) is neither. Mirrors _task_reservation_state in
// bin/_lib.sh, minus the stale/free distinction the callers don't need.
func taskLive(t state.Task, alive state.ProcAlive) (leased, locked bool) {
	if t.Lease {
		return true, false
	}
	if t.Pid != 0 && alive(t.Pid, t.ProcToken) {
		return false, true
	}
	return false, false
}

// reservationLive loads key's ledger entry and reports its live lease/lock state.
func reservationLive(home, key string) (leased, locked bool) {
	l, err := state.Load(state.FilePath(home))
	if err != nil {
		return false, false
	}
	t, ok := l.Tasks[key]
	if !ok {
		return false, false
	}
	return taskLive(t, proc.Alive)
}

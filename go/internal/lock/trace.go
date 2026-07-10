package lock

import (
	"fmt"
	"os"
	"path/filepath"
)

// traceEnter/traceExit are a gated concurrency self-check for the state lock:
// when SDEV_LOCK_TRACE is set, an O_EXCL HOLDER marker atomically detects a
// second holder inside the critical section and appends a DOUBLEHOLD line. Zero
// cost (one getenv) when unset; tests/state_interop.bats enables it to assert
// bash<->Go mutual exclusion, which cannot be checked from a pure-Go test.
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

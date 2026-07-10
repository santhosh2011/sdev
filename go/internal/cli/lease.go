package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/lock"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/proc"
	"github.com/santhosh2011/sdev/internal/state"
	"github.com/santhosh2011/sdev/internal/task"
)

// Lease implements `sdev lease <slug> [holder]`: a durable reservation that is
// never auto-reclaimed until released.
func Lease(args []string) int {
	slug, holder := "", os.Getenv("SDEV_LEASE_HOLDER")
	if len(args) > 0 {
		slug = args[0]
	}
	if len(args) > 1 {
		holder = args[1]
	}
	home := paths.Home()
	key, code := resolveKey(home, slug)
	if code != 0 {
		return code
	}
	if err := lock.With(state.Dir(home), func() error { return state.SetLease(home, key, holder) }); err != nil {
		return failErr(err)
	}
	suffix := ""
	if holder != "" {
		suffix = " to " + holder
	}
	logf("leased %s%s (durable - not auto-reclaimed until 'sdev release')", key, suffix)
	return 0
}

// Release implements `sdev release <slug>`: clears the lease and process-lock.
func Release(args []string) int {
	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}
	home := paths.Home()
	key, code := resolveKey(home, slug)
	if code != 0 {
		return code
	}
	if err := lock.With(state.Dir(home), func() error { return state.ClearReservation(home, key) }); err != nil {
		return failErr(err)
	}
	logf("released %s (lease + process-lock cleared)", key)
	return 0
}

// Hold implements `sdev hold <slug> [--pid <pid>]`: a process-lock recorded with
// the holder pid and its start-time token; it self-heals when the pid exits.
func Hold(args []string) int {
	slug, pidStr := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pid":
			if i+1 >= len(args) {
				return failMsg("--pid requires a value")
			}
			pidStr = args[i+1]
			i++
		default:
			if slug != "" {
				return failMsg("unexpected arg: " + args[i])
			}
			slug = args[i]
		}
	}
	// Default to the invoking shell (its death self-heals the lock).
	pid := os.Getppid()
	if pidStr != "" {
		n, err := strconv.Atoi(pidStr)
		if err != nil || n < 0 {
			return failMsg("invalid --pid '" + pidStr + "'")
		}
		pid = n
	}
	home := paths.Home()
	key, code := resolveKey(home, slug)
	if code != 0 {
		return code
	}
	procLock := state.ProcLock{Pid: pid, Token: proc.Token(pid)}
	if err := lock.With(state.Dir(home), func() error { return state.SetLock(home, key, procLock) }); err != nil {
		return failErr(err)
	}
	logf("process-lock on %s held by pid %d (self-heals when it exits)", key, pid)
	return 0
}

// resolveKey resolves a slug to its ledger key, or prints the not-found error and
// returns a non-zero exit code.
func resolveKey(home, slug string) (string, int) {
	dir, err := task.Resolve(home, config.ActiveProject(), slug)
	if err != nil {
		return "", failErr(err)
	}
	return task.Key(home, dir), 0
}

func failErr(err error) int {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	return 1
}

func failMsg(msg string) int {
	fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	return 1
}

// logf mirrors the bash log() helper's "[sdev] <msg>" line on stdout.
func logf(format string, args ...any) {
	fmt.Printf("[sdev] "+format+"\n", args...)
}

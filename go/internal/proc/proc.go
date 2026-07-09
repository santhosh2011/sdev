// Package proc checks process-lock liveness, mirroring _proc_alive / _proc_token
// in bin/_lib.sh: a pid is alive only if it exists and (when a token is given)
// its start-time signature still matches, so a reused pid reads as dead.
package proc

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// Alive reports whether pid is running and, when token is non-empty, its
// start-time signature matches.
func Alive(pid int, token string) bool {
	if pid == 0 {
		return false
	}
	// kill -0: nil means alive; EPERM means it exists but is not ours (still alive).
	if err := syscall.Kill(pid, 0); err != nil && err != syscall.EPERM {
		return false
	}
	if token != "" {
		return Token(pid) == token
	}
	return true
}

// Token returns pid's start-time signature (`ps -o lstart=`), whitespace-squeezed
// and trimmed, or "" if the process is gone.
func Token(pid int) string {
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.Join(strings.Fields(string(out)), " ")
}

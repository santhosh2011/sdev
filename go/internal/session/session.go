// Package session locates the per-terminal file that pins sdev's active project,
// mirroring the session_* helpers in bin/_lib.sh so a project pinned by bash is
// seen by Go and vice versa.
package session

import (
	"os"
	"path/filepath"
	"strings"
)

// keyEnvs are the terminal-identifying env vars, most specific first.
var keyEnvs = []string{"TERM_SESSION_ID", "SSH_TTY", "TTY"}

// Key identifies the current terminal: the first set of TERM_SESSION_ID / SSH_TTY
// / TTY, else "default", with path-unfriendly characters folded to underscores.
func Key() string {
	for _, env := range keyEnvs {
		if v := os.Getenv(env); v != "" {
			return sanitize(v)
		}
	}
	return "default"
}

// Dir is the per-terminal session directory under TMPDIR (or /tmp).
func Dir() string {
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	return filepath.Join(tmp, "sdev", Key())
}

// Pointer is the file holding this terminal's pinned project name.
func Pointer() string {
	return filepath.Join(Dir(), "active-project")
}

func sanitize(s string) string {
	return strings.NewReplacer("/", "_", " ", "_", ":", "_", ".", "_").Replace(s)
}

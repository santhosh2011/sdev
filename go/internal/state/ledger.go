// Package state reads (and, via writer.go, writes) sdev's central allocation
// ledger at $SDEV_HOME/state/state.yml. It mirrors the schema and semantics of
// the state functions in bin/_lib.sh so Go and bash interoperate on one ledger.
package state

import (
	"errors"
	"os"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Task is one ledger reservation, keyed by "<project>/<slug>".
type Task struct {
	Offset      int    `yaml:"offset"`
	CreatedAt   string `yaml:"created_at"`
	Lease       bool   `yaml:"lease"`
	LeaseHolder string `yaml:"lease_holder"`
	Pid         int    `yaml:"pid"`
	ProcToken   string `yaml:"proc_token"`
	Ephemeral   bool   `yaml:"ephemeral"`
}

// PoolEntry is one warm worktree returned by `sdev end --pool`.
type PoolEntry struct {
	Project    string `yaml:"project"`
	Repo       string `yaml:"repo"`
	RepoPath   string `yaml:"repo_path"`
	Source     string `yaml:"source"`
	Path       string `yaml:"path"`
	ReturnedAt string `yaml:"returned_at"`
}

// Ledger is the whole state file.
type Ledger struct {
	Version int             `yaml:"version"`
	Seeded  bool            `yaml:"seeded"`
	PoolSeq int             `yaml:"pool_seq"`
	Tasks   map[string]Task `yaml:"tasks"`
	Pool    []PoolEntry     `yaml:"pool"`
}

// ProcAlive reports whether a process-lock is live; injected so callers test
// without real processes.
type ProcAlive func(pid int, token string) bool

// Load reads the ledger at path. A missing file yields an empty ledger, not an
// error, mirroring the bash read helpers' `[[ -f ]] || return`.
func Load(path string) (*Ledger, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Ledger{Tasks: map[string]Task{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var l Ledger
	if err := yaml.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	if l.Tasks == nil {
		l.Tasks = map[string]Task{}
	}
	return &l, nil
}

// LeasedKeys returns the sorted keys of leased tasks (shown by `sdev ls` even
// when their workspace is gone).
func (l *Ledger) LeasedKeys() []string {
	keys := []string{}
	for k, t := range l.Tasks {
		if t.Lease {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// StatusLabel is the `sdev ls` annotation for a task: an "ephemeral" marker (when
// set) combined with the reservation state - "leased:holder", "lock:pid",
// "lock:stale", or "". Mirrors task_status_label in bin/_lib.sh.
func (t Task) StatusLabel(alive ProcAlive) string {
	out := ""
	if t.Ephemeral {
		out = "ephemeral"
	}
	if t.Lease {
		if t.LeaseHolder != "" {
			return join(out, "leased:"+t.LeaseHolder)
		}
		return join(out, "leased")
	}
	if t.Pid != 0 {
		if alive(t.Pid, t.ProcToken) {
			return join(out, "lock:"+strconv.Itoa(t.Pid))
		}
		return join(out, "lock:stale")
	}
	return out
}

// join concatenates two label fragments with a space, dropping empties (mirrors
// the bash `${out:+$out }` idiom).
func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}

package state

import "path/filepath"

// FilePath is the ledger file for a given SDEV_HOME.
func FilePath(home string) string { return filepath.Join(home, "state", "state.yml") }

// LockPath is the state lock (an atomic symlink whose target is the holder pid).
func LockPath(home string) string { return filepath.Join(home, "state", "lock") }

// Dir is the state directory.
func Dir(home string) string { return filepath.Join(home, "state") }

// PoolDir is where relocated warm worktrees live.
func PoolDir(home string) string { return filepath.Join(home, "state", "pool") }

// Package teardown implements the shared force-teardown of a task: the Go port of
// force_teardown_task in bin/_lib.sh. It stops the docker stack, removes every
// repo worktree and its task branch, drops the workspace directory, and frees the
// ledger entry. No archive, no pool, no pre-flight. It is driven by `sdev destroy`
// and (later) `sdev prune`.
package teardown

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/lock"
	"github.com/santhosh2011/sdev/internal/state"
)

// Ops are the side-effecting operations Force delegates, injected so teardown is
// unit-testable without real docker or git. The production wiring lives in the
// cli package; both operations are best-effort and swallow their own errors,
// mirroring the bash `|| true` on `compose down` and `git worktree remove`.
type Ops struct {
	// Down stops a task's docker stack given its workspace dir.
	Down func(taskDir string)
	// RemoveWorktree removes one repo worktree and deletes its task/<slug> branch.
	RemoveWorktree func(sourceDir, worktreeDir, branch string)
}

// Force tears down the task named by key ("<project>/<slug>", or a legacy bare
// "<slug>"). It is idempotent: a key whose workspace is already gone simply has
// its ledger entry freed. The only shared-state write - freeing the entry - is
// taken under the state lock.
func Force(home, key string, ops Ops) error {
	project, slug := splitKey(key)
	dir := filepath.Join(home, "projects", key)

	if fsutil.IsDir(dir) {
		ops.Down(dir)
		repos, err := config.Repos(home, project)
		if err != nil {
			return err
		}
		for _, repo := range repos {
			path, err := config.RepoPath(home, project, repo)
			if err != nil {
				return err
			}
			if path == "" {
				continue
			}
			worktree := filepath.Join(dir, path)
			if !fsutil.IsDir(worktree) {
				continue
			}
			ops.RemoveWorktree(config.RepoSourceDir(home, project, path), worktree, "task/"+slug)
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}

	return lock.With(state.Dir(home), func() error { return state.FreeTask(home, key) })
}

// splitKey splits a ledger key into project and slug. A key with no slash is a
// legacy flat task, whose project is "default". Mirrors the key-split in
// force_teardown_task.
func splitKey(key string) (project, slug string) {
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[:i], key[i+1:]
	}
	return "default", key
}

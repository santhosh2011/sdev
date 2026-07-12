package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/state"
	"github.com/santhosh2011/sdev/internal/teardown"
)

// Destroy implements `sdev destroy <slug> [--force]`: force-remove one task -
// worktree, branch, docker stack, offset, and ledger entry. No archive, no pool,
// no pre-flight. It refuses a task holding a live lease or process-lock unless
// --force, and resolves a workspace-less entry (e.g. a lease whose worktree is
// already gone) so it stays destroyable.
func Destroy(args []string) int {
	slug, force := "", false
	for _, a := range args {
		switch a {
		case "--force", "-f":
			force = true
		default:
			if slug != "" {
				return failMsg("unexpected arg: " + a)
			}
			slug = a
		}
	}
	if slug == "" {
		return failMsg("slug required")
	}

	home, project := paths.Home(), config.ActiveProject()
	key, ok := resolveDestroyKey(home, project, slug)
	if !ok {
		return failMsg(fmt.Sprintf("task '%s' not found (no workspace or ledger entry in project '%s')", slug, project))
	}

	if !force {
		switch leased, locked := reservationLive(home, key); {
		case leased:
			return failMsg(fmt.Sprintf("%s holds a live lease - 'sdev release %s' first, or pass --force", key, slug))
		case locked:
			return failMsg(fmt.Sprintf("%s holds a live process-lock - pass --force to destroy anyway", key))
		}
	}

	if err := teardown.Force(home, key, teardownOps()); err != nil {
		return failErr(err)
	}
	logf("destroyed %s (worktree + offset + ledger entry removed)", key)
	return 0
}

// resolveDestroyKey resolves a slug to its ledger key, preferring an on-disk
// workspace (namespaced then legacy flat) and falling back to a bare ledger
// entry. Mirrors the key resolution in bin/sdev's destroy block.
func resolveDestroyKey(home, project, slug string) (string, bool) {
	nsKey := project + "/" + slug
	if fsutil.IsDir(filepath.Join(home, "projects", nsKey)) {
		return nsKey, true
	}
	if fsutil.IsDir(filepath.Join(home, "projects", slug)) {
		return slug, true
	}
	l, err := state.Load(state.FilePath(home))
	if err != nil {
		return "", false
	}
	if _, ok := l.Tasks[nsKey]; ok {
		return nsKey, true
	}
	if _, ok := l.Tasks[slug]; ok {
		return slug, true
	}
	return "", false
}

// teardownOps wires the real docker and git side effects for teardown.Force. Both
// are best-effort: a failed `compose down` or `git worktree remove` must not stop
// the teardown, matching the bash `|| true` guards.
func teardownOps() teardown.Ops {
	return teardown.Ops{
		Down: func(taskDir string) {
			compose := filepath.Join(taskDir, "compose")
			if info, err := os.Stat(compose); err != nil || info.Mode()&0o111 == 0 {
				return
			}
			cmd := exec.Command("./compose", "down", "-v", "--remove-orphans")
			cmd.Dir = taskDir
			_ = cmd.Run()
		},
		RemoveWorktree: func(sourceDir, worktreeDir, branch string) {
			_ = exec.Command("git", "-C", sourceDir, "worktree", "remove", "--force", worktreeDir).Run()
			_ = exec.Command("git", "-C", sourceDir, "branch", "-D", branch).Run()
		},
	}
}

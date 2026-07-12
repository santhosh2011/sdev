package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/lock"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/proc"
	"github.com/santhosh2011/sdev/internal/state"
	"github.com/santhosh2011/sdev/internal/teardown"
)

// pruneOpts are the reclamation scope switches parsed from the command line.
type pruneOpts struct {
	PoolOnly  bool   // only drain the warm pool, leave task reservations alone
	DrainPool bool   // also drain the warm pool (remove cached worktrees)
	Project   string // restrict to one project's tasks / pool entries
}

// poolRef is a warm-pool worktree the plan will drain: its source clone and path.
type poolRef struct{ Source, Path string }

// prunePlan is the classified reclamation: what would be torn down, dropped, or
// drained, plus the protected tasks left untouched (shown for information).
type prunePlan struct {
	Ephemeral []string  // ephemeral tasks -> full teardown
	Ledger    []string  // abandoned entries (workspace gone) -> drop entry
	PoolStale []poolRef // pool paths whose worktree vanished -> drop entry
	PoolDrain []poolRef // pooled worktrees drained by --pool
	Protected []string  // live lease / live lock -> left untouched
}

// Prune implements `sdev prune`: safe, automatic-eligible reclamation of the
// state ledger. Dry-run by default; --apply performs it. Mirrors bin/prune.
func Prune(args []string) int {
	var opts pruneOpts
	apply := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			prunePrintUsage()
			return 0
		case "--apply", "-y", "--yes":
			apply = true
		case "--pool":
			opts.DrainPool = true
		case "--pool-only":
			opts.DrainPool, opts.PoolOnly = true, true
		case "--project":
			if i+1 >= len(args) {
				return failMsg("--project requires a value")
			}
			opts.Project = args[i+1]
			i++
		default:
			return failMsg("unknown flag: " + args[i])
		}
	}

	home := paths.Home()
	if err := state.Init(home); err != nil {
		return failErr(err)
	}
	l, err := state.Load(state.FilePath(home))
	if err != nil {
		return failErr(err)
	}
	plan := classifyPrune(home, l, opts, proc.Alive)
	printPrunePreview(plan)

	if !apply {
		fmt.Println()
		fmt.Println("(dry-run - nothing changed; pass --apply to perform)")
		return 0
	}
	return applyPrune(home, plan)
}

// classifyPrune sorts a ledger's tasks and pool entries into the reclamation
// plan. It is pure over the ledger plus on-disk workspace presence, so callers
// unit-test it by injecting a ledger, a home, and a proc-liveness function.
func classifyPrune(home string, l *state.Ledger, opts pruneOpts, alive state.ProcAlive) prunePlan {
	var p prunePlan
	if !opts.PoolOnly {
		for _, key := range sortedTaskKeys(l.Tasks) {
			if !pruneInScope(key, opts.Project) {
				continue
			}
			t := l.Tasks[key]
			switch leased, locked := taskLive(t, alive); {
			case leased:
				p.Protected = append(p.Protected, key+" [leased]")
			case locked:
				p.Protected = append(p.Protected, key+" [live lock]")
			case t.Ephemeral:
				p.Ephemeral = append(p.Ephemeral, key)
			case !fsutil.IsDir(filepath.Join(home, "projects", key)):
				p.Ledger = append(p.Ledger, key) // leaked offset, nothing on disk
			}
		}
	}
	for _, e := range l.Pool {
		if opts.Project != "" && e.Project != opts.Project {
			continue
		}
		switch {
		case !fsutil.IsDir(e.Path):
			p.PoolStale = append(p.PoolStale, poolRef{e.Source, e.Path})
		case opts.DrainPool:
			p.PoolDrain = append(p.PoolDrain, poolRef{e.Source, e.Path})
		}
	}
	return p
}

// applyPrune performs the plan: full teardown of ephemeral tasks, dropping
// abandoned ledger entries, and draining stale/pooled worktrees. Each shared
// write is taken under the state lock inside its helper.
func applyPrune(home string, plan prunePlan) int {
	fmt.Println()
	fmt.Println("=== prune apply ===")
	for _, key := range plan.Ephemeral {
		if err := teardown.Force(home, key, teardownOps()); err != nil {
			return failErr(err)
		}
		logf("reclaimed ephemeral %s (worktree + offset freed)", key)
	}
	for _, key := range plan.Ledger {
		if err := lock.With(state.Dir(home), func() error { return state.FreeTask(home, key) }); err != nil {
			return failErr(err)
		}
		logf("dropped abandoned ledger entry %s (offset freed)", key)
	}
	for _, e := range plan.PoolStale {
		drainPoolEntry(home, e)
		logf("dropped stale pool entry %s", e.Path)
	}
	for _, e := range plan.PoolDrain {
		drainPoolEntry(home, e)
		logf("drained pooled worktree %s", e.Path)
	}
	fmt.Println("prune: done")
	return 0
}

// drainPoolEntry removes a pooled worktree (via git when its source is a real
// repo, else rm -rf) and drops its ledger entry. Best-effort like the bash
// drain_pool_entry: a failed git remove falls back to a plain delete.
func drainPoolEntry(home string, e poolRef) {
	if hasGitRepo(e.Source) {
		if err := exec.Command("git", "-C", e.Source, "worktree", "remove", "--force", e.Path).Run(); err != nil {
			_ = os.RemoveAll(e.Path)
		}
	} else {
		_ = os.RemoveAll(e.Path)
	}
	_ = lock.With(state.Dir(home), func() error { return state.DropPool(home, e.Path) })
}

func printPrunePreview(plan prunePlan) {
	fmt.Println("=== prune preview ===")
	printPruneSection("ephemeral tasks to reclaim (worktree + offset + entry):", plan.Ephemeral)
	printPruneSection("abandoned ledger entries to drop (free leaked offset):", plan.Ledger)
	printPruneSection("stale pool entries to drop (worktree already gone):", poolPaths(plan.PoolStale))
	printPruneSection("warm-pool worktrees to drain (free disk):", poolPaths(plan.PoolDrain))
	printPruneSection("protected (left untouched):", plan.Protected)
	if len(plan.Ephemeral)+len(plan.Ledger)+len(plan.PoolStale)+len(plan.PoolDrain) == 0 {
		fmt.Println("  nothing to reclaim")
	}
}

func printPruneSection(header string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Println(header)
	for _, it := range items {
		fmt.Println("  - " + it)
	}
}

func poolPaths(refs []poolRef) []string {
	paths := make([]string, len(refs))
	for i, r := range refs {
		paths[i] = r.Path
	}
	return paths
}

// hasGitRepo reports whether src is a git checkout (a .git dir) or a linked
// worktree (a .git file), mirroring the guard in drain_pool_entry.
func hasGitRepo(src string) bool {
	if src != "" && src != "null" && fsutil.IsDir(filepath.Join(src, ".git")) {
		return true
	}
	return fsutil.FileExists(filepath.Join(src, ".git"))
}

func pruneInScope(key, project string) bool {
	if project == "" {
		return true
	}
	return key == project || strings.HasPrefix(key, project+"/")
}

func sortedTaskKeys(tasks map[string]state.Task) []string {
	keys := make([]string, 0, len(tasks))
	for k := range tasks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func prunePrintUsage() {
	fmt.Println(`Usage: sdev prune [--apply|-y] [--pool] [--pool-only] [--project <name>]

Reclaims ephemeral tasks, abandoned ledger entries (leaked offsets), and stale
warm-pool entries. A live-leased or live-locked task is never touched.

  --apply, -y     actually perform the reclamation (default: dry-run preview)
  --pool          also drain the warm pool (remove cached worktrees, free disk)
  --pool-only     only drain the warm pool; leave task reservations alone
  --project NAME  restrict to one project's tasks / pool entries
  -h, --help      show this help`)
}

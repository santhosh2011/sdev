package cli

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/santhosh2011/sdev/internal/dockerx"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/proc"
	"github.com/santhosh2011/sdev/internal/state"
)

// aliveRowFmt renders one alive-task line: label, offset, nginx URL, status, and
// an optional reservation annotation.
const aliveRowFmt = "  %-28s offset=%-3s nginx=http://localhost:%-5s [%s]%s\n"

// printLSHuman renders the human `sdev ls` listing: alive tasks, workspace-less
// leases, the warm pool, archived tasks, and orphan volumes. It reuses the same
// on-disk walkers as the --json path and layers the ledger-derived detail
// (reservation labels, leases, pool) on top.
func printLSHuman(home, scope string) {
	ledger, err := state.Load(state.FilePath(home))
	if err != nil {
		ledger = &state.Ledger{Tasks: map[string]state.Task{}}
	}

	fmt.Println("=== Alive tasks ===")
	alive := collectAlive(home, scope, dockerx.RunningByComposeProject)
	for _, a := range alive {
		printTaskHuman(a, ledger)
	}
	if len(alive) == 0 {
		fmt.Println("  (none)")
	}
	printLeasesHuman(home, scope, ledger)

	fmt.Println()
	fmt.Println("=== Warm pool ===")
	if !printPoolHuman(scope, ledger) {
		fmt.Println("  (none)")
	}

	fmt.Println()
	fmt.Println("=== Archived tasks ===")
	archived := collectArchived(home)
	for _, a := range archived {
		fmt.Printf("  %-28s archived %s\n", a.Label, dashIfEmpty(a.Date))
	}
	if len(archived) == 0 {
		fmt.Println("  (none)")
	}

	fmt.Println()
	fmt.Println("=== Orphan docker volumes ===")
	orphans := findOrphans(home, dockerx.Volumes)
	for _, vol := range orphans {
		fmt.Println("  " + vol)
	}
	if len(orphans) == 0 {
		fmt.Println("  (none)")
	}
}

func printTaskHuman(a aliveTask, ledger *state.Ledger) {
	suffix := ""
	if resv := taskLabel(ledger, a.Key); resv != "" {
		suffix = " [" + resv + "]"
	}
	fmt.Printf(aliveRowFmt, a.Label, dashIfZero(a.Offset), dashIfZero(a.Nginx), statusHuman(a.Running), suffix)
}

func printLeasesHuman(home, scope string, ledger *state.Ledger) {
	shown := false
	for _, key := range ledger.LeasedKeys() {
		if fsutil.IsDir(filepath.Join(home, projectsRoot, key)) {
			continue // already listed among alive tasks
		}
		if scope != "" && key != scope && !strings.HasPrefix(key, scope+"/") {
			continue
		}
		if !shown {
			fmt.Println()
			fmt.Println("=== Leases (no workspace) ===")
			shown = true
		}
		fmt.Printf("  %-28s [%s]\n", key, taskLabel(ledger, key))
	}
}

func printPoolHuman(scope string, ledger *state.Ledger) bool {
	found := false
	for _, p := range ledger.Pool {
		if p.Path == "" || (scope != "" && p.Project != scope) {
			continue
		}
		found = true
		status := "ok"
		if !fsutil.IsDir(p.Path) {
			status = "MISSING"
		}
		fmt.Printf("  %-28s %s [%s]\n", p.Project+"/"+p.Repo, p.Path, status)
	}
	return found
}

// taskLabel is the reservation annotation for a ledger key, or "" if the key is
// absent (mirrors task_status_label's has(key) guard).
func taskLabel(ledger *state.Ledger, key string) string {
	t, ok := ledger.Tasks[key]
	if !ok {
		return ""
	}
	return t.StatusLabel(proc.Alive)
}

func statusHuman(running int) string {
	if running > 0 {
		return fmt.Sprintf("running (%d containers)", running)
	}
	return "stopped"
}

func dashIfZero(n int) string {
	if n == 0 {
		return "-"
	}
	return strconv.Itoa(n)
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

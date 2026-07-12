package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/dockerx"
	"github.com/santhosh2011/sdev/internal/jsonout"
	"github.com/santhosh2011/sdev/internal/paths"
)

// dashboardDescription is the agent-facing one-liner shown at the top of the
// axi dashboard (the no-arg surface), matching the axi-family convention.
const dashboardDescription = "Agent-ergonomic manager for isolated parallel dev workspaces - one git worktree + its own docker stack per task, on unique ports. Prefer this over hand-managing branches, ports, or stacks."

// Dashboard is the no-arg axi surface: a terse, agent-scannable snapshot of the
// fleet (bin, description, per-project counts) plus next-step affordances. It
// reuses the status tally. `sdev --help` still prints the full command list.
func Dashboard(args []string) int {
	report, err := BuildStatusReport(paths.Home(), config.ActiveProject(), dockerx.RunningByComposeProject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sdev: %v\n", err)
		return 1
	}
	bin, _ := os.Executable()
	fmt.Print(renderDashboard(report, bin))
	return 0
}

// renderDashboard formats the fleet report as the axi dashboard. Projects are
// ordered most-active first (running, then task count, then name) so the busy
// work surfaces at a glance.
func renderDashboard(r jsonout.StatusReport, binPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bin: %s\n", binPath)
	fmt.Fprintf(&b, "description: %s\n", dashboardDescription)
	fmt.Fprintf(&b, "home: %s   active: %s\n", r.SdevHome, r.ActiveProject)
	fmt.Fprintf(&b, "fleet: %d projects · %d tasks · %d running\n", r.Totals.Projects, r.Totals.Tasks, r.Totals.Running)

	projects := append([]jsonout.ProjectCount(nil), r.Projects...)
	sort.SliceStable(projects, func(i, j int) bool {
		if projects[i].Running != projects[j].Running {
			return projects[i].Running > projects[j].Running
		}
		if projects[i].Tasks != projects[j].Tasks {
			return projects[i].Tasks > projects[j].Tasks
		}
		return projects[i].Name < projects[j].Name
	})
	fmt.Fprintf(&b, "projects[%d]:\n", len(projects))
	for _, p := range projects {
		fmt.Fprintf(&b, "  %-12s tasks=%-3d running=%d\n", p.Name, p.Tasks, p.Running)
	}

	b.WriteString("help[3]:\n")
	b.WriteString("  Run `sdev ls` for per-task detail (url, ports, status)\n")
	b.WriteString("  Run `sdev start <slug>` to create-or-resume a task and boot it\n")
	b.WriteString("  Run `sdev --help` for the full command list\n")
	return b.String()
}

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/dockerx"
	"github.com/santhosh2011/sdev/internal/envfile"
	"github.com/santhosh2011/sdev/internal/jsonout"
	"github.com/santhosh2011/sdev/internal/paths"
)

// statusNext is the agent next-step hint carried in the JSON payload; it matches
// the bash implementation verbatim.
var statusNext = []string{"sdev ls --json  # per-task detail"}

// RunningCounter counts running containers for a compose project name. It is
// injected into BuildStatusReport so tests run without docker (DIP).
type RunningCounter func(composeProject string) int

// Status implements `sdev status [--json]`.
func Status(args []string) int {
	jsonOut := false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
		}
	}

	report, err := BuildStatusReport(paths.Home(), config.ActiveProject(), dockerx.RunningByComposeProject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sdev status: %v\n", err)
		return 1
	}

	if jsonOut {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "sdev status: %v\n", err)
			return 1
		}
		return 0
	}
	printStatusHuman(report)
	return 0
}

// BuildStatusReport walks every defined project under home and tallies task and
// running-stack counts. Running containers are counted via the injected counter.
func BuildStatusReport(home, active string, running RunningCounter) (jsonout.StatusReport, error) {
	names, err := config.Projects(home)
	if err != nil {
		return jsonout.StatusReport{}, err
	}

	projects := make([]jsonout.ProjectCount, 0, len(names))
	totalTasks, totalRunning := 0, 0
	for _, name := range names {
		tasks, run := tallyProject(home, name, running)
		projects = append(projects, jsonout.ProjectCount{Name: name, Tasks: tasks, Running: run})
		totalTasks += tasks
		totalRunning += run
	}

	return jsonout.StatusReport{
		SdevHome:      home,
		ActiveProject: active,
		Projects:      projects,
		Totals: jsonout.StatusTotals{
			Projects: len(names),
			Tasks:    totalTasks,
			Running:  totalRunning,
		},
		Next: statusNext,
	}, nil
}

// tallyProject counts task dirs (those holding a .env) and running containers
// for a single project. A missing project dir counts as zero.
func tallyProject(home, project string, running RunningCounter) (tasks, run int) {
	dir := filepath.Join(home, "projects", project)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		envPath := filepath.Join(dir, entry.Name(), ".env")
		if _, err := os.Stat(envPath); err != nil {
			continue
		}
		tasks++
		compose := envfile.Value(envPath, "COMPOSE_PROJECT_NAME")
		if compose == "" {
			compose = entry.Name()
		}
		run += running(compose)
	}
	return tasks, run
}

func printStatusHuman(r jsonout.StatusReport) {
	fmt.Println("sdev status")
	fmt.Printf("  SDEV_HOME: %s\n", r.SdevHome)
	fmt.Printf("  active project: %s\n", r.ActiveProject)
	for _, p := range r.Projects {
		fmt.Printf("  %s\ttasks=%d\trunning=%d\n", p.Name, p.Tasks, p.Running)
	}
	fmt.Printf("  totals: projects=%d tasks=%d running=%d\n",
		r.Totals.Projects, r.Totals.Tasks, r.Totals.Running)
	fmt.Println("  Next: sdev ls   # per-task detail")
}

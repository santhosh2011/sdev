package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh2011/sdev/internal/jsonout"
)

// zeroRunning is an injected RunningCounter for hermetic tests (no docker).
func zeroRunning(string) int { return 0 }

func TestBuildStatusReportCountsTasksPerProject(t *testing.T) {
	home := seedStatusFixture(t)

	report, err := BuildStatusReport(home, "acme", zeroRunning)
	if err != nil {
		t.Fatalf("BuildStatusReport: %v", err)
	}
	if report.Totals.Projects != 2 {
		t.Fatalf("Totals.Projects = %d, want 2", report.Totals.Projects)
	}
	if report.Totals.Tasks != 3 {
		t.Fatalf("Totals.Tasks = %d, want 3", report.Totals.Tasks)
	}
	if got := tasksFor(report, "acme"); got != 2 {
		t.Fatalf("acme tasks = %d, want 2", got)
	}
}

func TestBuildStatusReportRunningIsZeroWithoutContainers(t *testing.T) {
	home := seedStatusFixture(t)

	report, err := BuildStatusReport(home, "acme", zeroRunning)
	if err != nil {
		t.Fatalf("BuildStatusReport: %v", err)
	}
	if report.Totals.Running != 0 {
		t.Fatalf("Totals.Running = %d, want 0", report.Totals.Running)
	}
}

func tasksFor(r jsonout.StatusReport, name string) int {
	for _, p := range r.Projects {
		if p.Name == name {
			return p.Tasks
		}
	}
	return -1
}

// seedStatusFixture mirrors tests/status.bats: project "default" (1 task) and
// project "acme" (2 tasks), each task dir holding a .env.
func seedStatusFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, name := range []string{"default", "acme"} {
		writeProjectFile(t, home, name)
	}
	writeTaskEnv(t, home, "default", "a")
	writeTaskEnv(t, home, "acme", "b")
	writeTaskEnv(t, home, "acme", "c")
	return home
}

func writeProjectFile(t *testing.T, home, name string) {
	t.Helper()
	dir := filepath.Join(home, "core", "projects.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yml"), []byte("conf_prefix: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTaskEnv(t *testing.T, home, project, slug string) {
	t.Helper()
	dir := filepath.Join(home, "projects", project, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "PORT_OFFSET=10\nNGINX_HOST_PORT=8090\nCOMPOSE_PROJECT_NAME=" + project + "_" + slug + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

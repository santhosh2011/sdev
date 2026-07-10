package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/santhosh2011/sdev/internal/jsonout"
)

func noVolumes() []string { return nil }

func TestBuildLSReportListsAliveWithURLAndTotals(t *testing.T) {
	home := seedLSFixture(t)

	r := BuildLSReport(home, "", zeroRunning, noVolumes)

	if r.Totals.Alive != 2 {
		t.Fatalf("Totals.Alive = %d, want 2", r.Totals.Alive)
	}
	b := aliveFor(r, "acme/b")
	if b == nil {
		t.Fatal("acme/b not listed")
	}
	if b.NginxPort != 8100 || b.URL != "http://localhost:8100/" {
		t.Fatalf("acme/b = %+v, want port 8100 + url", b)
	}
	if r.Project != nil {
		t.Fatalf("unscoped project = %v, want nil", *r.Project)
	}
}

func TestBuildLSReportIncludesArchivedDate(t *testing.T) {
	home := seedLSFixture(t)

	r := BuildLSReport(home, "", zeroRunning, noVolumes)

	if got := archivedFor(r, "acme/old"); got != "2026-06-01" {
		t.Fatalf("acme/old archived = %q, want 2026-06-01", got)
	}
}

func TestBuildLSReportScopedToProject(t *testing.T) {
	home := seedLSFixture(t)

	r := BuildLSReport(home, "acme", zeroRunning, noVolumes)

	if r.Totals.Alive != 1 || r.Alive[0].Task != "acme/b" {
		t.Fatalf("scoped alive = %+v, want only acme/b", r.Alive)
	}
	if r.Project == nil || *r.Project != "acme" {
		t.Fatalf("scoped project = %v, want acme", r.Project)
	}
}

func TestBuildLSReportOrphanVolumesAlwaysArray(t *testing.T) {
	home := seedLSFixture(t)

	r := BuildLSReport(home, "", zeroRunning, noVolumes)

	if r.OrphanVolumes == nil {
		t.Fatal("orphan_volumes must be a non-nil array")
	}
}

func aliveFor(r jsonout.LSReport, task string) *jsonout.LSAlive {
	for i := range r.Alive {
		if r.Alive[i].Task == task {
			return &r.Alive[i]
		}
	}
	return nil
}

func archivedFor(r jsonout.LSReport, task string) string {
	for _, a := range r.Archived {
		if a.Task == task {
			return a.Archived
		}
	}
	return ""
}

// seedLSFixture mirrors tests/ls_json.bats: default/a (port 8090), acme/b (port
// 8100), and archived acme/old dated 2026-06-01.
func seedLSFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	writeProjectFile(t, home, "default")
	writeProjectFile(t, home, "acme")
	writeTaskEnvPort(t, home, "default", "a", 8090)
	writeTaskEnvPort(t, home, "acme", "b", 8100)
	writeArchived(t, home, "acme", "old", "2026-06-01")
	return home
}

func writeTaskEnvPort(t *testing.T, home, project, slug string, port int) {
	t.Helper()
	dir := filepath.Join(home, "projects", project, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "PORT_OFFSET=10\nNGINX_HOST_PORT=" + strconv.Itoa(port) + "\nCOMPOSE_PROJECT_NAME=" + project + "_" + slug + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeArchived(t *testing.T, home, project, slug, date string) {
	t.Helper()
	dir := filepath.Join(home, "projects", "_archive", project, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "- archive_date: " + date + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ARCHIVE_INFO.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

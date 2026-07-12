package cli

import (
	"strings"
	"testing"

	"github.com/santhosh2011/sdev/internal/jsonout"
)

func sampleReport() jsonout.StatusReport {
	return jsonout.StatusReport{
		SdevHome:      "/home",
		ActiveProject: "scdi",
		Projects: []jsonout.ProjectCount{
			{Name: "scdi", Tasks: 5, Running: 0},
			{Name: "pdmt", Tasks: 2, Running: 4},
			{Name: "spnr", Tasks: 2, Running: 0},
		},
		Totals: jsonout.StatusTotals{Projects: 3, Tasks: 9, Running: 4},
	}
}

func TestRenderDashboardShowsBinAndFleet(t *testing.T) {
	out := renderDashboard(sampleReport(), "/x/bin/sdev")

	if !strings.Contains(out, "bin: /x/bin/sdev") {
		t.Fatalf("missing bin line:\n%s", out)
	}
	if !strings.Contains(out, "fleet: 3 projects · 9 tasks · 4 running") {
		t.Fatalf("missing fleet line:\n%s", out)
	}
	if !strings.Contains(out, "description:") || !strings.Contains(out, "active: scdi") {
		t.Fatalf("missing description/active:\n%s", out)
	}
}

func TestRenderDashboardSortsByActivity(t *testing.T) {
	out := renderDashboard(sampleReport(), "/x/bin/sdev")
	// Compare within the project list, since "scdi" also appears in the header.
	list := out[strings.Index(out, "projects["):]

	// The busiest project (pdmt, running=4) must be listed before the idle ones.
	if strings.Index(list, "pdmt") > strings.Index(list, "scdi") {
		t.Fatalf("expected pdmt (running) before scdi:\n%s", out)
	}
}

func TestRenderDashboardHasHelpAffordances(t *testing.T) {
	out := renderDashboard(sampleReport(), "/x/bin/sdev")

	if !strings.Contains(out, "help[3]:") {
		t.Fatalf("missing help block:\n%s", out)
	}
	if !strings.Contains(out, "sdev ls") || !strings.Contains(out, "sdev start") {
		t.Fatalf("missing help affordances:\n%s", out)
	}
}

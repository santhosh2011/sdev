package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh2011/sdev/internal/state"
)

// livePid is the one pid the test's alive func treats as running.
const livePid = 999

func aliveOnlyLivePid(pid int, _ string) bool { return pid == livePid }

func TestClassifyPruneBucketsReclaimableTasks(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, "widget/eph", "widget/real") // eph + real exist on disk; gone does not
	l := ledgerWith(map[string]state.Task{
		"widget/eph":  {Ephemeral: true},
		"widget/gone": {},
		"widget/real": {},
	})

	p := classifyPrune(home, l, pruneOpts{}, aliveOnlyLivePid)

	if len(p.Ephemeral) != 1 || p.Ephemeral[0] != "widget/eph" {
		t.Fatalf("Ephemeral = %v, want [widget/eph]", p.Ephemeral)
	}
	if len(p.Ledger) != 1 || p.Ledger[0] != "widget/gone" {
		t.Fatalf("Ledger = %v, want [widget/gone]", p.Ledger)
	}
	if len(p.Protected) != 0 {
		t.Fatalf("Protected = %v, want none (real is a normal live task)", p.Protected)
	}
}

func TestClassifyPruneProtectsLeasedAndLocked(t *testing.T) {
	home := t.TempDir()
	mkdirs(t, home, "widget/keep", "widget/live")
	l := ledgerWith(map[string]state.Task{
		"widget/keep": {Lease: true},
		"widget/live": {Pid: livePid, ProcToken: "t"},
	})

	p := classifyPrune(home, l, pruneOpts{}, aliveOnlyLivePid)

	if !containsStr(p.Protected, "widget/keep [leased]") {
		t.Fatalf("Protected = %v, want widget/keep [leased]", p.Protected)
	}
	if !containsStr(p.Protected, "widget/live [live lock]") {
		t.Fatalf("Protected = %v, want widget/live [live lock]", p.Protected)
	}
	if len(p.Ephemeral)+len(p.Ledger) != 0 {
		t.Fatal("a protected task must not be reclaimed")
	}
}

func TestClassifyPrunePoolStaleAndDrain(t *testing.T) {
	home := t.TempDir()
	live := filepath.Join(home, "pool", "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	l := &state.Ledger{Pool: []state.PoolEntry{
		{Project: "widget", Source: "/src/a", Path: filepath.Join(home, "pool", "gone")},
		{Project: "widget", Source: "/src/b", Path: live},
	}}

	p := classifyPrune(home, l, pruneOpts{DrainPool: true}, aliveOnlyLivePid)

	if len(p.PoolStale) != 1 || !strings.HasSuffix(p.PoolStale[0].Path, "gone") {
		t.Fatalf("PoolStale = %+v, want the vanished worktree", p.PoolStale)
	}
	if len(p.PoolDrain) != 1 || p.PoolDrain[0].Path != live {
		t.Fatalf("PoolDrain = %+v, want the live pooled worktree", p.PoolDrain)
	}
}

func TestClassifyPrunePoolOnlySkipsTasks(t *testing.T) {
	home := t.TempDir()
	l := ledgerWith(map[string]state.Task{"widget/eph": {Ephemeral: true}})

	p := classifyPrune(home, l, pruneOpts{PoolOnly: true}, aliveOnlyLivePid)

	if len(p.Ephemeral) != 0 {
		t.Fatalf("--pool-only must skip task reclaim, got %v", p.Ephemeral)
	}
}

func ledgerWith(tasks map[string]state.Task) *state.Ledger {
	return &state.Ledger{Tasks: tasks}
}

func mkdirs(t *testing.T, home string, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := os.MkdirAll(filepath.Join(home, "projects", k), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

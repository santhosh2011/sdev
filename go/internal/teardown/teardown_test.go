package teardown

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh2011/sdev/internal/state"
)

type removeCall struct{ source, worktree, branch string }

// recorder captures the injected side effects so tests assert what Force
// delegated without touching real docker or git.
type recorder struct {
	downs   []string
	removes []removeCall
}

func (r *recorder) ops() Ops {
	return Ops{
		Down:           func(dir string) { r.downs = append(r.downs, dir) },
		RemoveWorktree: func(src, wt, br string) { r.removes = append(r.removes, removeCall{src, wt, br}) },
	}
}

func TestForceTearsDownWorktreeAndDir(t *testing.T) {
	home := seedWidgetTask(t, "held")
	dir := filepath.Join(home, "projects", "widget", "held")
	r := &recorder{}

	if err := Force(home, "widget/held", r.ops()); err != nil {
		t.Fatalf("Force: %v", err)
	}

	if len(r.removes) != 1 {
		t.Fatalf("RemoveWorktree calls = %d, want 1", len(r.removes))
	}
	got := r.removes[0]
	if got.worktree != filepath.Join(dir, "svc") || got.branch != "task/held" {
		t.Fatalf("RemoveWorktree = %+v, want worktree=%s branch=task/held", got, filepath.Join(dir, "svc"))
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("task dir still present: %v", err)
	}
}

func TestForceFreesLedgerEntry(t *testing.T) {
	home := seedWidgetTask(t, "held")

	if err := Force(home, "widget/held", (&recorder{}).ops()); err != nil {
		t.Fatalf("Force: %v", err)
	}

	l, err := state.Load(state.FilePath(home))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := l.Tasks["widget/held"]; ok {
		t.Fatal("ledger entry not freed")
	}
}

func TestForceIdempotentWhenNoWorkspace(t *testing.T) {
	home := seedWidgetTask(t, "held")
	os.RemoveAll(filepath.Join(home, "projects", "widget", "held")) // workspace already gone
	r := &recorder{}

	if err := Force(home, "widget/held", r.ops()); err != nil {
		t.Fatalf("Force: %v", err)
	}

	if len(r.removes) != 0 || len(r.downs) != 0 {
		t.Fatalf("delegated side effects with no workspace: %+v", r)
	}
	l, _ := state.Load(state.FilePath(home))
	if _, ok := l.Tasks["widget/held"]; ok {
		t.Fatal("ledger entry not freed")
	}
}

// seedWidgetTask builds a home with the widget project registry, a task
// workspace holding one repo worktree, and a ledger entry for it.
func seedWidgetTask(t *testing.T, slug string) string {
	t.Helper()
	home := t.TempDir()
	reg := filepath.Join(home, "core", "projects.d")
	if err := os.MkdirAll(reg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reg, "widget.yml"),
		[]byte("repos:\n  api: { path: svc }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "projects", "widget", slug, "svc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.Init(home); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state.FilePath(home),
		[]byte("tasks:\n  widget/"+slug+":\n    offset: 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

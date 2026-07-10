package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh2011/sdev/internal/state"
)

func TestDestroyRefusesLeasedWithoutForce(t *testing.T) {
	home := seedCliTask(t, "held", true)
	dir := filepath.Join(home, "projects", "widget", "held")

	if code := Destroy([]string{"held"}); code == 0 {
		t.Fatal("Destroy of a leased task without --force should fail")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("refused destroy removed the workspace: %v", err)
	}
}

func TestDestroyForceRemovesLeasedTask(t *testing.T) {
	home := seedCliTask(t, "held", true)
	dir := filepath.Join(home, "projects", "widget", "held")

	if code := Destroy([]string{"held", "--force"}); code != 0 {
		t.Fatalf("Destroy --force = %d, want 0", code)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("workspace still present: %v", err)
	}
	if ledgerHas(t, home, "widget/held") {
		t.Fatal("ledger entry not freed")
	}
}

func TestDestroyUnknownSlugFails(t *testing.T) {
	seedCliTask(t, "held", false)
	if code := Destroy([]string{"ghost"}); code == 0 {
		t.Fatal("Destroy of an unknown slug should fail")
	}
}

func TestDestroyFreesBareLedgerEntry(t *testing.T) {
	home := seedCliTask(t, "held", false)
	os.RemoveAll(filepath.Join(home, "projects", "widget", "held")) // workspace gone, entry remains

	if code := Destroy([]string{"held"}); code != 0 {
		t.Fatalf("Destroy of a bare entry = %d, want 0", code)
	}
	if ledgerHas(t, home, "widget/held") {
		t.Fatal("bare ledger entry not freed")
	}
}

// seedCliTask points SDEV_HOME/SDEV_PROJECT at a fresh home holding the widget
// project registry, a workspace for slug, and a ledger entry (optionally leased).
func seedCliTask(t *testing.T, slug string, leased bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("SDEV_HOME", home)
	t.Setenv("SDEV_PROJECT", "widget")

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
	entry := "tasks:\n  widget/" + slug + ":\n    offset: 10\n"
	if leased {
		entry += "    lease: true\n    lease_holder: owner\n"
	}
	if err := os.WriteFile(state.FilePath(home), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func ledgerHas(t *testing.T, home, key string) bool {
	t.Helper()
	l, err := state.Load(state.FilePath(home))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, ok := l.Tasks[key]
	return ok
}

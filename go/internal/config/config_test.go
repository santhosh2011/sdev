package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectsReturnsSortedBasenames(t *testing.T) {
	home := t.TempDir()
	mkProject(t, home, "default")
	mkProject(t, home, "acme")

	got, err := Projects(home)
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	want := []string{"acme", "default"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Projects = %v, want %v", got, want)
	}
}

func TestProjectsEmptyWhenNoRegistry(t *testing.T) {
	got, err := Projects(t.TempDir())
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Projects = %v, want empty", got)
	}
}

func mkProject(t *testing.T, home, name string) {
	t.Helper()
	dir := filepath.Join(home, "core", "projects.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yml"), []byte("conf_prefix: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

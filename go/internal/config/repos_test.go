package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReposReturnsSortedKeys(t *testing.T) {
	home := t.TempDir()
	writeProjectRepos(t, home, "widget", "ui: { path: web }\n  api: { path: svc }")

	got, err := Repos(home, "widget")
	if err != nil {
		t.Fatalf("Repos: %v", err)
	}
	if len(got) != 2 || got[0] != "api" || got[1] != "ui" {
		t.Fatalf("Repos = %v, want [api ui]", got)
	}
}

func TestRepoPathReturnsConfiguredPath(t *testing.T) {
	home := t.TempDir()
	writeProjectRepos(t, home, "widget", "api: { path: svc }")

	got, err := RepoPath(home, "widget", "api")
	if err != nil {
		t.Fatalf("RepoPath: %v", err)
	}
	if got != "svc" {
		t.Fatalf("RepoPath = %q, want svc", got)
	}
}

func TestRepoSourceDirPrefersNamespaced(t *testing.T) {
	home := t.TempDir()
	nsSrc := filepath.Join(home, "core", "widget", "svc")
	if err := os.MkdirAll(nsSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RepoSourceDir(home, "widget", "svc"); got != nsSrc {
		t.Fatalf("RepoSourceDir = %q, want %q", got, nsSrc)
	}
}

func TestRepoSourceDirFallsBackToFlat(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, "core", "svc")
	if got := RepoSourceDir(home, "widget", "svc"); got != want {
		t.Fatalf("RepoSourceDir = %q, want %q", got, want)
	}
}

func TestEffectiveProjectFileFallsBackToGlobal(t *testing.T) {
	home := t.TempDir()
	global := filepath.Join(home, "core", ".task-config.yml")
	if err := os.MkdirAll(filepath.Dir(global), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(global, []byte("repos: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := EffectiveProjectFile(home, "widget"); got != global {
		t.Fatalf("EffectiveProjectFile = %q, want %q", got, global)
	}
}

// writeProjectRepos writes core/projects.d/<project>.yml with the given repos map
// body (indented under a `repos:` block).
func writeProjectRepos(t *testing.T, home, project, reposBody string) {
	t.Helper()
	dir := filepath.Join(home, "core", "projects.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "repos:\n  " + reposBody + "\n"
	if err := os.WriteFile(filepath.Join(dir, project+".yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

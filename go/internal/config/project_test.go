package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectFlagWins(t *testing.T) {
	t.Setenv("SDEV_PROJECT", "envproj")
	if got := ResolveProject(t.TempDir(), "flagproj"); got != "flagproj" {
		t.Fatalf("ResolveProject = %q, want flagproj", got)
	}
}

func TestResolveProjectFallsBackToEnvThenDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TMPDIR", t.TempDir()) // no stray session pointer
	t.Setenv("SDEV_PROJECT", "envproj")
	if got := ResolveProject(home, ""); got != "envproj" {
		t.Fatalf("ResolveProject = %q, want envproj", got)
	}
	t.Setenv("SDEV_PROJECT", "")
	if got := ResolveProject(home, ""); got != "default" {
		t.Fatalf("ResolveProject = %q, want default", got)
	}
}

func TestRequireProjectKnownAndDefaultOK(t *testing.T) {
	home := t.TempDir()
	mkProject(t, home, "acme")

	if p, err := RequireProject(home, "acme"); err != nil || p != "acme" {
		t.Fatalf("RequireProject(acme) = %q, %v", p, err)
	}
	if _, err := RequireProject(home, "default"); err != nil {
		t.Fatalf("RequireProject(default): %v", err)
	}
}

func TestRequireProjectUnknownErrors(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "core", "projects.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := RequireProject(home, "ghost"); err == nil {
		t.Fatal("RequireProject(ghost) should error")
	}
}

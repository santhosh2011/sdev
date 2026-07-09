package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveNamespaced(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "projects", "acme", "b"))

	got, err := Resolve(home, "acme", "b")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "projects", "acme", "b"); got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
}

func TestResolveLegacyFlat(t *testing.T) {
	home := t.TempDir()
	mustMkdir(t, filepath.Join(home, "projects", "solo"))

	got, err := Resolve(home, "default", "solo")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "projects", "solo"); got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
}

func TestResolveMissingErrors(t *testing.T) {
	if _, err := Resolve(t.TempDir(), "acme", "nope"); err == nil {
		t.Fatal("Resolve missing: want error, got nil")
	}
}

func TestResolveEmptySlugErrors(t *testing.T) {
	if _, err := Resolve(t.TempDir(), "acme", ""); err == nil {
		t.Fatal("Resolve empty slug: want error, got nil")
	}
}

func TestKeyStripsProjectsPrefix(t *testing.T) {
	home := "/h"
	got := Key(home, filepath.Join(home, "projects", "acme", "b"))
	if want := filepath.Join("acme", "b"); got != want {
		t.Fatalf("Key = %q, want %q", got, want)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

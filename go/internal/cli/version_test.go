package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadVersionFileStripsAnnotation(t *testing.T) {
	dir := t.TempDir()
	// The repo VERSION carries a release-please annotation comment.
	os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.2.3 # x-release-please-version\n"), 0o644)

	if got := readVersionFile(filepath.Join(dir, "VERSION")); got != "1.2.3" {
		t.Fatalf("readVersionFile = %q, want 1.2.3", got)
	}
}

func TestReadVersionFileCleanBareVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.0.0\n"), 0o644)

	if got := readVersionFile(filepath.Join(dir, "VERSION")); got != "1.0.0" {
		t.Fatalf("readVersionFile = %q, want 1.0.0", got)
	}
}

func TestReadVersionFileMissingIsEmpty(t *testing.T) {
	if got := readVersionFile(filepath.Join(t.TempDir(), "nope")); got != "" {
		t.Fatalf("readVersionFile(missing) = %q, want empty", got)
	}
}

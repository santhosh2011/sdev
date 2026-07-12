package cli

import "testing"

func TestRunNoArgsPrintsUsage(t *testing.T) {
	t.Setenv("SDEV_HOME", t.TempDir())
	if got := Run(nil); got != 0 {
		t.Fatalf("Run(nil) = %d, want 0 (usage)", got)
	}
}

func TestRunUnknownFlagIsError(t *testing.T) {
	t.Setenv("SDEV_HOME", t.TempDir())
	if got := Run([]string{"--bogus"}); got != 1 {
		t.Fatalf("Run(--bogus) = %d, want 1", got)
	}
}

func TestRunVersionSucceeds(t *testing.T) {
	t.Setenv("SDEV_HOME", t.TempDir())
	if got := Run([]string{"version"}); got != 0 {
		t.Fatalf("Run(version) = %d, want 0", got)
	}
}

func TestRunHelpSucceeds(t *testing.T) {
	t.Setenv("SDEV_HOME", t.TempDir())
	if got := Run([]string{"help"}); got != 0 {
		t.Fatalf("Run(help) = %d, want 0", got)
	}
}

package cli

import "testing"

func TestRunNoArgsShowsDashboard(t *testing.T) {
	t.Setenv("SDEV_HOME", t.TempDir())
	// No args is the axi dashboard now (exit 0); `sdev help` prints full usage.
	if got := Run(nil); got != 0 {
		t.Fatalf("Run(nil) = %d, want 0 (dashboard)", got)
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

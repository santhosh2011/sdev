package cli

import "testing"

func TestRunNoArgsIsUsageError(t *testing.T) {
	if got := Run(nil); got != 2 {
		t.Fatalf("Run(nil) = %d, want 2", got)
	}
}

func TestRunUnknownSubcommandIsUsageError(t *testing.T) {
	if got := Run([]string{"bogus"}); got != 2 {
		t.Fatalf("Run(bogus) = %d, want 2", got)
	}
}

func TestRunVersionSucceeds(t *testing.T) {
	if got := Run([]string{"version"}); got != 0 {
		t.Fatalf("Run(version) = %d, want 0", got)
	}
}

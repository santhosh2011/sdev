package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValueReadsKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	writeFile(t, p, "PORT_OFFSET=10\nCOMPOSE_PROJECT_NAME=acme_b\n")
	if got := Value(p, "COMPOSE_PROJECT_NAME"); got != "acme_b" {
		t.Fatalf("Value = %q, want acme_b", got)
	}
}

func TestValueMissingKeyReturnsEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	writeFile(t, p, "PORT_OFFSET=10\n")
	if got := Value(p, "NOPE"); got != "" {
		t.Fatalf("Value = %q, want empty", got)
	}
}

func TestValueMissingFileReturnsEmpty(t *testing.T) {
	if got := Value("/no/such/.env", "X"); got != "" {
		t.Fatalf("Value = %q, want empty", got)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

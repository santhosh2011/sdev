package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupHooksInstallsSessionStartHook(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if code := Setup([]string{"hooks"}); code != 0 {
		t.Fatalf("Setup(hooks) = %d, want 0", code)
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("settings not written: %v", err)
	}
	if !strings.Contains(string(data), "SessionStart") || !strings.Contains(string(data), "startup|resume") {
		t.Fatalf("hook not installed:\n%s", data)
	}
}

func TestSetupHooksIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	Setup([]string{"hooks"})
	Setup([]string{"hooks"}) // second run must not duplicate

	data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings not valid JSON: %v", err)
	}
	hooks := settings["hooks"].(map[string]any)
	if got := len(hooks["SessionStart"].([]any)); got != 1 {
		t.Fatalf("SessionStart entries = %d, want 1 (idempotent)", got)
	}
}

func TestSetupHooksPreservesExistingSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"model":"opus","hooks":{}}`), 0o644)

	Setup([]string{"hooks"})

	data, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if !strings.Contains(string(data), `"model"`) || !strings.Contains(string(data), "opus") {
		t.Fatalf("existing settings clobbered:\n%s", data)
	}
}

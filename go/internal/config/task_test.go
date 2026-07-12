package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBasePortPrefersProjectOverride(t *testing.T) {
	home := t.TempDir()
	writeGlobal(t, home, "defaults:\n  base_ports: { api: 8291 }\n")
	writeProject(t, home, "widget", "base_ports: { api: 9000 }\n")

	if got := BasePort(home, "widget", "api"); got != 9000 {
		t.Fatalf("BasePort = %d, want project override 9000", got)
	}
}

func TestBasePortFallsBackToGlobalDefault(t *testing.T) {
	home := t.TempDir()
	writeGlobal(t, home, "defaults:\n  base_ports: { db: 3306 }\n")
	writeProject(t, home, "widget", "conf_prefix: x\n")

	if got := BasePort(home, "widget", "db"); got != 3306 {
		t.Fatalf("BasePort = %d, want global 3306", got)
	}
}

func TestHooksEnabledDefaultsTrueDisabledByFalse(t *testing.T) {
	home := t.TempDir()
	writeProject(t, home, "on", "conf_prefix: x\n")
	writeProject(t, home, "off", "hooks: false\n")

	if !HooksEnabled(home, "on") {
		t.Fatal("hooks should default enabled")
	}
	if HooksEnabled(home, "off") {
		t.Fatal("hooks: false should disable")
	}
}

func TestResolveProfileForNewRejectsInvalid(t *testing.T) {
	home := t.TempDir()
	if _, err := ResolveProfileForNew(home, "prod"); err == nil {
		t.Fatal("prod is not a valid profile; want error")
	}
	if p, err := ResolveProfileForNew(home, "dev"); err != nil || p != "dev" {
		t.Fatalf("ResolveProfileForNew(dev) = %q, %v", p, err)
	}
}

func writeGlobal(t *testing.T, home, body string) {
	t.Helper()
	dir := filepath.Join(home, "core")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".task-config.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProject(t *testing.T, home, project, body string) {
	t.Helper()
	dir := filepath.Join(home, "core", "projects.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, project+".yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

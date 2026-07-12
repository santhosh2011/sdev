package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Setup implements `sdev setup <target>`: wire sdev into agent tooling. Today it
// supports `hooks` - a Claude Code SessionStart hook that injects the fleet
// dashboard as ambient context. Mirrors the axi-family `setup` convention.
func Setup(args []string) int {
	if len(args) == 0 || args[0] != "hooks" {
		fmt.Println("Usage: sdev setup hooks")
		fmt.Println("Install a SessionStart hook so `sdev` (the fleet dashboard) is injected as")
		fmt.Println("ambient context at the start of every Claude Code session.")
		return 0
	}
	return setupHooks()
}

// setupHooks idempotently merges an sdev SessionStart hook into the user's global
// ~/.claude/settings.json, preserving every other setting. It aborts rather than
// clobber a settings file that is present but not valid JSON.
func setupHooks() int {
	bin, err := os.Executable()
	if err != nil || bin == "" {
		bin = "sdev"
	}
	command := bin + " axi-hook" // emits the dashboard as a SessionStart envelope
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")

	settings := map[string]any{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if json.Unmarshal(data, &settings) != nil {
			return failMsg(settingsPath + " is present but not valid JSON - fix or remove it, then re-run")
		}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	// Drop any prior sdev hook (incl. an older bare-`sdev` one) so re-running
	// normalizes to the current command instead of duplicating.
	sessionStart := dropSdevHooks(hooks["SessionStart"], bin)

	hooks["SessionStart"] = append(sessionStart, map[string]any{
		"matcher": "startup|resume",
		"hooks":   []any{map[string]any{"type": "command", "command": command}},
	})
	settings["hooks"] = hooks

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return failErr(err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return failErr(err)
	}
	if err := os.WriteFile(settingsPath, append(data, '\n'), 0o644); err != nil {
		return failErr(err)
	}
	logf("installed sdev SessionStart hook into %s (fleet dashboard injected each session)", settingsPath)
	return 0
}

// dropSdevHooks returns the SessionStart entries with any sdev-owned entry
// removed - one whose command is the sdev binary, with or without a subcommand.
func dropSdevHooks(raw any, bin string) []any {
	entries, _ := raw.([]any)
	kept := make([]any, 0, len(entries))
	for _, entry := range entries {
		if entryRunsSdev(entry, bin) {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

func entryRunsSdev(entry any, bin string) bool {
	m, _ := entry.(map[string]any)
	inner, _ := m["hooks"].([]any)
	for _, h := range inner {
		hm, _ := h.(map[string]any)
		cmd, _ := hm["command"].(string)
		if cmd == bin || strings.HasPrefix(cmd, bin+" ") {
			return true
		}
	}
	return false
}

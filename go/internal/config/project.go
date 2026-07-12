package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh2011/sdev/internal/session"
)

// ResolveProject resolves the active project using bin/_lib.sh's precedence:
// an explicit flag, then $SDEV_PROJECT, then the terminal's pinned project, then
// the local/global default_project, else "default".
func ResolveProject(home, flag string) string {
	if flag != "" {
		return flag
	}
	if p := os.Getenv("SDEV_PROJECT"); p != "" {
		return p
	}
	if data, err := os.ReadFile(session.Pointer()); err == nil {
		if p := strings.TrimSpace(string(data)); p != "" {
			return p
		}
	}
	local := filepath.Join(home, "core", ".task-config.local.yml")
	if d := readDoc(local).Defaults.DefaultProject; d != "" {
		return d
	}
	if d := readDoc(globalConfig(home)).Defaults.DefaultProject; d != "" {
		return d
	}
	return "default"
}

// RequireProject validates that a resolved project is known: it has a registry
// file, or it is the implicit single-project "default". Mirrors require_project.
func RequireProject(home, project string) (string, error) {
	if project == "" {
		return "", fmt.Errorf("no project resolved")
	}
	if fileReadable(projectConfigFile(home, project)) || project == "default" {
		return project, nil
	}
	known, _ := Projects(home)
	return "", fmt.Errorf("unknown project '%s' (known: %sdefault)", project, listWithTrailingSpace(known))
}

// projectConfigFile is a project's dedicated registry file (may not exist).
func projectConfigFile(home, project string) string {
	return filepath.Join(home, "core", "projects.d", project+".yml")
}

func fileReadable(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func listWithTrailingSpace(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, " ") + " "
}

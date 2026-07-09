// Package config reads sdev's project registry (core/projects.d).
package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultProject is the implicit project when none is resolved by the router.
const defaultProject = "default"

// Projects returns the names of all defined projects: the basenames (without the
// .yml suffix) of core/projects.d/*.yml, sorted for deterministic output.
func Projects(home string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(home, "core", "projects.d", "*.yml"))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, strings.TrimSuffix(filepath.Base(m), ".yml"))
	}
	sort.Strings(names)
	return names, nil
}

// ActiveProject returns the resolved active project. The bash router bin/sdev
// always exports SDEV_PROJECT before dispatch, so we trust it here; a direct
// invocation with no env falls back to the default project.
func ActiveProject() string {
	if p := os.Getenv("SDEV_PROJECT"); p != "" {
		return p
	}
	return defaultProject
}

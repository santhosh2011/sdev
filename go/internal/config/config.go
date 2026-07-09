// Package config reads sdev's project registry (core/projects.d).
package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultProject is the implicit project when none is resolved by the router.
const defaultProject = "default"

// defaultPortStep is the fallback offset step when the config omits it.
const defaultPortStep = 10

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

// PortStep is the offset step (.defaults.port_step in the global config), or 10.
func PortStep(home string) int {
	data, err := os.ReadFile(filepath.Join(home, "core", ".task-config.yml"))
	if err != nil {
		return defaultPortStep
	}
	var cfg struct {
		Defaults struct {
			PortStep int `yaml:"port_step"`
		} `yaml:"defaults"`
	}
	if err := yaml.Unmarshal(data, &cfg); err == nil && cfg.Defaults.PortStep > 0 {
		return cfg.Defaults.PortStep
	}
	return defaultPortStep
}

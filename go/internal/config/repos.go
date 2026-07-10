package config

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// globalConfigName is the legacy single-project registry used when a project has
// no dedicated core/projects.d/<project>.yml file.
const globalConfigName = ".task-config.yml"

// repoEntry is the per-repo shape sdev reads from a project registry: only the
// worktree-relative path matters to teardown.
type repoEntry struct {
	Path string `yaml:"path"`
}

type registry struct {
	Repos map[string]repoEntry `yaml:"repos"`
}

// EffectiveProjectFile is the registry that actually holds a project's repos: its
// dedicated core/projects.d/<project>.yml if present, else the legacy global
// core/.task-config.yml. Mirrors effective_project_file in bin/_lib.sh.
func EffectiveProjectFile(home, project string) string {
	pf := filepath.Join(home, "core", "projects.d", project+".yml")
	if _, err := os.Stat(pf); err == nil {
		return pf
	}
	return filepath.Join(home, "core", globalConfigName)
}

// Repos returns a project's repo keys, sorted for deterministic iteration.
func Repos(home, project string) ([]string, error) {
	reg, err := loadRegistry(home, project)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(reg.Repos))
	for k := range reg.Repos {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

// RepoPath returns the worktree-relative path configured for a repo, or "" if the
// repo is unknown.
func RepoPath(home, project, repo string) (string, error) {
	reg, err := loadRegistry(home, project)
	if err != nil {
		return "", err
	}
	return reg.Repos[repo].Path, nil
}

// RepoSourceDir resolves a repo's source clone: the namespaced
// core/<project>/<repoPath> when it exists, else the flat core/<repoPath>.
// Mirrors repo_source_dir in bin/_lib.sh.
func RepoSourceDir(home, project, repoPath string) string {
	ns := filepath.Join(home, "core", project, repoPath)
	if info, err := os.Stat(ns); err == nil && info.IsDir() {
		return ns
	}
	return filepath.Join(home, "core", repoPath)
}

// loadRegistry parses a project's effective registry file. A missing file yields
// an empty registry, mirroring how the bash yq reads degrade to no repos.
func loadRegistry(home, project string) (registry, error) {
	data, err := os.ReadFile(EffectiveProjectFile(home, project))
	if os.IsNotExist(err) {
		return registry{}, nil
	}
	if err != nil {
		return registry{}, err
	}
	var reg registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return registry{}, err
	}
	return reg, nil
}

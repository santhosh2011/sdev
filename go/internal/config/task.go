package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/paths"
	"gopkg.in/yaml.v3"
)

// validProfiles are the environment profiles a task may target.
var validProfiles = []string{"local", "dev", "staging"}

// projectDoc is the subset of a project/global registry that task creation reads.
// A project file and the global .task-config.yml share enough shape to reuse it.
type projectDoc struct {
	ConfPrefix          string                    `yaml:"conf_prefix"`
	Template            string                    `yaml:"template"`
	StackServices       []string                  `yaml:"stack_services"`
	BasePorts           map[string]int            `yaml:"base_ports"`
	Hooks               *bool                     `yaml:"hooks"`
	DefaultShellService string                    `yaml:"default_shell_service"`
	Repos               map[string]map[string]any `yaml:"repos"`
	Defaults            struct {
		DefaultEnv string         `yaml:"default_env"`
		BasePorts  map[string]int `yaml:"base_ports"`
	} `yaml:"defaults"`
}

// RepoBase returns a repo's configured default base branch (.repos.<repo>.default_base).
func RepoBase(home, project, repo string) string {
	return repoField(home, project, repo, "default_base")
}

// RepoAttr returns an arbitrary repo attribute as a string, or "" if unset.
func RepoAttr(home, project, repo, attr string) string {
	return repoField(home, project, repo, attr)
}

// BasePort returns a service's base host port: the project's own base_ports
// override, else the global defaults.base_ports.
func BasePort(home, project, service string) int {
	if v, ok := readDoc(EffectiveProjectFile(home, project)).BasePorts[service]; ok && v != 0 {
		return v
	}
	return readDoc(globalConfig(home)).Defaults.BasePorts[service]
}

// StackServices returns the compose services for a project: its own list, else
// the global default list.
func StackServices(home, project string) []string {
	if s := readDoc(EffectiveProjectFile(home, project)).StackServices; len(s) > 0 {
		return s
	}
	return readDoc(globalConfig(home)).StackServices
}

// TemplatePath is the docker-compose template for a project: the project's own
// (resolved under SDEV_HOME) if set, else the bundled default under the install.
func TemplatePath(home, project string) string {
	if t := readDoc(EffectiveProjectFile(home, project)).Template; t != "" {
		return filepath.Join(home, t)
	}
	return filepath.Join(paths.Install(), "bin", "templates", "docker-compose.yml.tmpl")
}

// UsesDefaultTemplate reports whether a project relies on the bundled template
// (and thus opts into the default template's service-pruning behavior).
func UsesDefaultTemplate(home, project string) bool {
	return readDoc(EffectiveProjectFile(home, project)).Template == ""
}

// HooksEnabled reports whether task creation should wire the sdev Claude hooks;
// only an explicit `hooks: false` disables them.
func HooksEnabled(home, project string) bool {
	h := readDoc(EffectiveProjectFile(home, project)).Hooks
	return h == nil || *h
}

// ConfPrefix returns a project's conf-file prefix, defaulting to "app".
func ConfPrefix(home, project string) string {
	if p := readDoc(EffectiveProjectFile(home, project)).ConfPrefix; p != "" {
		return p
	}
	return "app"
}

// ShellService returns the compose service `sdev shell` execs into, defaulting to "api".
func ShellService(home, project string) string {
	if s := readDoc(EffectiveProjectFile(home, project)).DefaultShellService; s != "" {
		return s
	}
	return "api"
}

// DefaultEnv is the fallback environment profile: the local override config, then
// the global config, then "local".
func DefaultEnv(home string) string {
	local := filepath.Join(home, "core", ".task-config.local.yml")
	if v := readDoc(local).Defaults.DefaultEnv; v != "" {
		return v
	}
	if v := readDoc(globalConfig(home)).Defaults.DefaultEnv; v != "" {
		return v
	}
	return "local"
}

// ProfileConfFile resolves a task's app.env target: confs/<project>/<prefix>.<profile>.env
// when the per-project conf dir exists, else the legacy flat confs/<prefix>.<profile>.env.
func ProfileConfFile(home, profile, project string) string {
	name := ConfPrefix(home, project) + "." + profile + ".env"
	if pdir := filepath.Join(home, "confs", project); fsutil.IsDir(pdir) {
		return filepath.Join(pdir, name)
	}
	return filepath.Join(home, "confs", name)
}

// ValidProfile reports whether p is an allowed environment profile.
func ValidProfile(p string) bool {
	for _, v := range validProfiles {
		if v == p {
			return true
		}
	}
	return false
}

// ValidProfiles returns the allowed profiles (for error messages).
func ValidProfiles() []string { return append([]string(nil), validProfiles...) }

// ResolveProfileForNew resolves the env profile for a new task: the requested
// one, else the project/global default. It errors on an invalid profile.
func ResolveProfileForNew(home, requested string) (string, error) {
	p := requested
	if p == "" {
		p = DefaultEnv(home)
	}
	if !ValidProfile(p) {
		return "", fmt.Errorf("invalid env profile '%s' (allowed: %v)", p, validProfiles)
	}
	return p, nil
}

func globalConfig(home string) string {
	return filepath.Join(home, "core", globalConfigName)
}

func repoField(home, project, repo, field string) string {
	doc := readDoc(EffectiveProjectFile(home, project))
	r, ok := doc.Repos[repo]
	if !ok {
		return ""
	}
	return scalarString(r[field])
}

// readDoc parses a registry file into projectDoc; a missing/invalid file yields a
// zero doc so callers fall through to their defaults.
func readDoc(path string) projectDoc {
	var doc projectDoc
	data, err := os.ReadFile(path)
	if err != nil {
		return doc
	}
	_ = yaml.Unmarshal(data, &doc)
	return doc
}

func scalarString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	default:
		return fmt.Sprint(x)
	}
}

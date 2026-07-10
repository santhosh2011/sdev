package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/paths"
	"gopkg.in/yaml.v3"
)

const npmrcBody = `# Do NOT run ` + "`npm install`" + ` here - node_modules is symlinked to the source repo.
# Installing here pollutes the shared source. Add deps in the source repo.
`

// writeTaskFiles generates every file in a freshly-worktreed task dir: .env,
// the app.env symlink, the compose template + wrapper, node_modules links,
// CLAUDE.md, and the Claude settings. Mirrors the second half of bin/new-task.
func writeTaskFiles(home, project, slug, profile string, offset int, selected []string, taskDir string) error {
	if err := writeEnvFile(home, project, slug, profile, offset, taskDir); err != nil {
		return err
	}
	if err := linkAppEnv(home, project, profile, taskDir); err != nil {
		return err
	}
	if err := copyTemplates(home, project, taskDir); err != nil {
		return err
	}
	if err := applyDefaultTemplate(home, project, selected, taskDir); err != nil {
		return err
	}
	linkNodeModules(home, project, selected, taskDir)
	if err := os.MkdirAll(filepath.Join(taskDir, "knowledge"), 0o755); err != nil {
		return err
	}
	if err := writeClaudeMd(home, project, slug, profile, taskDir); err != nil {
		return err
	}
	return writeClaudeSettings(home, project, taskDir)
}

func writeEnvFile(home, project, slug, profile string, offset int, taskDir string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "COMPOSE_PROJECT_NAME=%s-%s\n", project, slug)
	fmt.Fprintf(&b, "PORT_OFFSET=%d\n", offset)
	fmt.Fprintf(&b, "APP_ENV=%s\n", profile)
	for _, svc := range config.StackServices(home, project) {
		fmt.Fprintf(&b, "%s=%d\n", hostPortVar(svc), config.BasePort(home, project, svc)+offset)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "# App runtime config comes from ./app.env (symlink -> %s)\n", config.ProfileConfFile(home, profile, project))
	return os.WriteFile(filepath.Join(taskDir, ".env"), []byte(b.String()), 0o644)
}

func linkAppEnv(home, project, profile, taskDir string) error {
	conf := config.ProfileConfFile(home, profile, project)
	link := filepath.Join(taskDir, "app.env")
	_ = os.Remove(link)
	if err := os.Symlink(conf, link); err != nil {
		return err
	}
	if !fsutil.FileExists(conf) {
		logf("warning: %s does not exist - create it before 'sdev up'", conf)
	}
	return nil
}

func copyTemplates(home, project, taskDir string) error {
	if err := copyFile(config.TemplatePath(home, project), filepath.Join(taskDir, "docker-compose.yml"), 0o644); err != nil {
		return err
	}
	return copyFile(filepath.Join(paths.Install(), "bin", "templates", "compose.tmpl"), filepath.Join(taskDir, "compose"), 0o755)
}

// applyDefaultTemplate prunes UI/nginx services from the bundled compose template
// when the task has no ui repo, or installs the nginx conf when it does. Only the
// bundled default template opts into this; a project-supplied template is left
// untouched.
func applyDefaultTemplate(home, project string, selected []string, taskDir string) error {
	if !config.UsesDefaultTemplate(home, project) {
		return nil
	}
	if contains(selected, "ui") {
		return copyFile(filepath.Join(paths.Install(), "bin", "templates", "nginx.conf.tmpl"),
			filepath.Join(taskDir, "nginx.conf"), 0o644)
	}
	return pruneComposeUI(filepath.Join(taskDir, "docker-compose.yml"))
}

func linkNodeModules(home, project string, selected []string, taskDir string) {
	for _, repo := range selected {
		if config.RepoAttr(home, project, repo, "link_node_modules") != "true" {
			continue
		}
		rp, _ := config.RepoPath(home, project, repo)
		srcNM := filepath.Join(config.RepoSourceDir(home, project, rp), "node_modules")
		dst := filepath.Join(taskDir, rp, "node_modules")
		if fsutil.IsDir(srcNM) {
			_ = os.Remove(dst)
			_ = os.Symlink(srcNM, dst)
		} else {
			logf("warning: %s missing - run npm install in the source repo first", srcNM)
		}
		_ = os.WriteFile(filepath.Join(taskDir, rp, ".npmrc"), []byte(npmrcBody), 0o644)
	}
}

func writeClaudeMd(home, project, slug, profile, taskDir string) error {
	tmpl, err := os.ReadFile(filepath.Join(paths.Install(), "bin", "templates", "CLAUDE.md.tmpl"))
	if err != nil {
		return err
	}
	envPort := func(key string) string {
		if v := readEnvValue(taskDir, key); v != "" {
			return v
		}
		return "N/A"
	}
	out := string(tmpl)
	for placeholder, value := range map[string]string{
		"__SLUG__":            slug,
		"__PROJECT__":         project,
		"__APP_ENV__":         profile,
		"__NGINX_HOST_PORT__": envPort("NGINX_HOST_PORT"),
		"__API_HOST_PORT__":   envPort("API_HOST_PORT"),
		"__UI_HOST_PORT__":    envPort("UI_HOST_PORT"),
		"__DB_HOST_PORT__":    envPort("DB_HOST_PORT"),
		"__REDIS_HOST_PORT__": envPort("REDIS_HOST_PORT"),
	} {
		out = strings.ReplaceAll(out, placeholder, value)
	}
	if profile == "staging" {
		out = "> \U0001f534 **STAGING ENVIRONMENT** - this task targets your real staging environment.\n" +
			"> It can read/write REAL staging data. Double-check before running mutations.\n\n" + out
	}
	return os.WriteFile(filepath.Join(taskDir, "CLAUDE.md"), []byte(out), 0o644)
}

// hookCommand is one hook invocation; hookMatcher binds it to a tool matcher.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookMatcher struct {
	Matcher string        `json:"matcher"`
	Hooks   []hookCommand `json:"hooks"`
}

// claudeSettings is the .claude/settings.local.json shape when hooks are wired.
// Field/hook order is fixed to keep output stable across runs.
type claudeSettings struct {
	AdditionalDirectories []string `json:"additionalDirectories"`
	Hooks                 struct {
		SessionStart []hookMatcher `json:"SessionStart"`
		PreToolUse   []hookMatcher `json:"PreToolUse"`
		PostToolUse  []hookMatcher `json:"PostToolUse"`
	} `json:"hooks"`
}

func writeClaudeSettings(home, project, taskDir string) error {
	settings := filepath.Join(taskDir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		return err
	}
	hooksDir := filepath.Join(paths.Install(), "claude", "hooks")
	if !config.HooksEnabled(home, project) || !fsutil.IsDir(hooksDir) {
		return copyFile(filepath.Join(paths.Install(), "bin", "templates", "settings.local.json.tmpl"), settings, 0o644)
	}

	matcher := func(m, script string) []hookMatcher {
		return []hookMatcher{{Matcher: m, Hooks: []hookCommand{{Type: "command", Command: filepath.Join(hooksDir, script)}}}}
	}
	var s claudeSettings
	s.AdditionalDirectories = []string{}
	s.Hooks.SessionStart = matcher("startup|resume", "sdev-session-context")
	s.Hooks.PreToolUse = matcher("Bash", "sdev-staging-guard")
	s.Hooks.PostToolUse = matcher("Edit|Write", "sdev-edit-reminder")
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settings, append(data, '\n'), 0o644)
}

func printNextSteps(project, slug string, offset int, taskDir string) {
	na := func(v string) string {
		if v == "" {
			return "N/A"
		}
		return v
	}
	nginx := na(readEnvValue(taskDir, "NGINX_HOST_PORT"))
	fmt.Printf(`
next steps:
  cd projects/%s/%s
  ./compose up
  # open http://localhost:%s/
  claude

ports allocated (offset=%d): nginx=%s api=%s ui=%s db=%s redis=%s
`, project, slug, nginx, offset, nginx,
		na(readEnvValue(taskDir, "API_HOST_PORT")), na(readEnvValue(taskDir, "UI_HOST_PORT")),
		na(readEnvValue(taskDir, "DB_HOST_PORT")), na(readEnvValue(taskDir, "REDIS_HOST_PORT")))
}

// hostPortVar maps a service name to its .env host-port variable, e.g. "db" ->
// "DB_HOST_PORT", "ui-web" -> "UI_WEB_HOST_PORT". Mirrors the bash tr expression.
func hostPortVar(service string) string {
	return strings.ToUpper(strings.ReplaceAll(service, "-", "_")) + "_HOST_PORT"
}

func readEnvValue(taskDir, key string) string {
	data, err := os.ReadFile(filepath.Join(taskDir, ".env"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	return ""
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

// pruneComposeUI removes the ui/nginx services and the ui-node-modules volume
// from a compose file, mirroring the yq deletions new-task runs for the default
// template when a task has no ui repo.
func pruneComposeUI(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 1 {
		root := doc.Content[0]
		deleteMapKey(mapValue(root, "services"), "ui")
		deleteMapKey(mapValue(root, "services"), "nginx")
		deleteMapKey(mapValue(root, "volumes"), "ui-node-modules")
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// mapValue returns the value node for key in a mapping node, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// deleteMapKey removes the key/value pair for key from a mapping node.
func deleteMapKey(m *yaml.Node, key string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

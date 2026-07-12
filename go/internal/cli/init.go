package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/session"
)

// repoSpec is one repo gathered by the init wizard.
type repoSpec struct{ name, base, role string }

// Init implements `sdev init`: an interactive wizard that writes a project
// registry, wires each repo's source (clone or symlink), and seeds a local conf.
// Mirrors bin/init.
func Init(args []string) int {
	home := paths.Home()
	ensureHome(home)
	p := newPrompter()

	project := p.ask("Project name (kebab-case)", "")
	if !validSlug(project) {
		return failMsg("invalid project name '" + project + "' - kebab-case, 1-50 chars")
	}
	projFile := filepath.Join(home, "core", "projects.d", project+".yml")
	if exists(projFile) {
		return failMsg("project '" + project + "' already exists at " + projFile)
	}

	confPrefix := p.ask("Conf prefix", "app")
	if !validToken(confPrefix) {
		return failMsg("invalid conf prefix '" + confPrefix + "' (allowed: letters, digits, . _ -)")
	}
	shellService := p.ask("Shell service (compose service for `sdev shell`)", "api")
	if !validToken(shellService) {
		return failMsg("invalid shell service '" + shellService + "'")
	}

	stack, code := askStack(p)
	if code != 0 {
		return code
	}

	repos, code := askRepos(p, home, project)
	if code != 0 {
		return code
	}
	if len(repos) == 0 {
		return failMsg("no repos added - aborting")
	}

	if err := os.WriteFile(projFile, []byte(renderProjectYAML(confPrefix, shellService, stack, repos)), 0o644); err != nil {
		return failErr(err)
	}
	logf("wrote %s", projFile)

	if err := seedConf(home, project, confPrefix); err != nil {
		return failErr(err)
	}
	pinProject(home, project)
	printInitNextSteps(project)
	return 0
}

func askStack(p *prompter) ([]string, int) {
	line := p.ask("Stack services (host-exposed, comma/space separated; blank = inherit default)", "")
	var stack []string
	for _, s := range splitCommaSpace(line) {
		if !validToken(s) {
			return nil, failMsg("invalid stack service '" + s + "'")
		}
		stack = append(stack, s)
	}
	return stack, 0
}

func askRepos(p *prompter, home, project string) ([]repoSpec, int) {
	var repos []repoSpec
	for {
		name := p.ask("Repo name (blank to finish)", "")
		if name == "" {
			break
		}
		if !validSlug(name) {
			fmt.Fprintln(os.Stderr, "  invalid repo name; skipping")
			continue
		}
		spec := p.ask("  git URL to clone, or path to an existing local repo, for '"+name+"'", "")
		base := p.ask("  default base branch", "main")
		if !validRef(base) {
			return nil, failMsg("invalid base branch '" + base + "'")
		}
		role := p.ask("  compose role", name)
		if !validToken(role) {
			return nil, failMsg("invalid compose role '" + role + "'")
		}
		verb, err := addRepoSource(home, project, name, spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  skipping repo '%s'\n", name)
			continue
		}
		logf("%s repo '%s'", verb, name)
		repos = append(repos, repoSpec{name: name, base: base, role: role})
	}
	return repos, 0
}

// renderProjectYAML writes the registry exactly as bin/init does, line by line.
func renderProjectYAML(confPrefix, shellService string, stack []string, repos []repoSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "conf_prefix: %s\n", confPrefix)
	fmt.Fprintf(&b, "default_shell_service: %s\n", shellService)
	if len(stack) > 0 {
		fmt.Fprintf(&b, "stack_services: [%s]\n", strings.Join(stack, ", "))
	}
	b.WriteString("repos:\n")
	for _, r := range repos {
		fmt.Fprintf(&b, "  %s:\n", r.name)
		fmt.Fprintf(&b, "    path: %s\n", r.name)
		fmt.Fprintf(&b, "    default_base: %s\n", r.base)
		fmt.Fprintf(&b, "    compose_role: %s\n", r.role)
		b.WriteString("    link_node_modules: false\n")
	}
	return b.String()
}

func seedConf(home, project, confPrefix string) error {
	dir := filepath.Join(home, "confs", project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(dir, confPrefix+".local.env")
	if fileExists(dest) {
		logf("wrote %s", dest)
		return nil
	}
	example := filepath.Join(paths.Install(), "confs", "example", "app.local.env.example")
	if fileExists(example) {
		if err := copyFile(example, dest, 0o644); err != nil {
			return err
		}
	} else if err := os.WriteFile(dest, []byte("APP_ENV=local\n"), 0o644); err != nil {
		return err
	}
	logf("wrote %s", dest)
	return nil
}

// pinProject pins the project for this terminal (best-effort).
func pinProject(home, project string) {
	_ = os.MkdirAll(session.Dir(), 0o755)
	_ = os.WriteFile(session.Pointer(), []byte(project+"\n"), 0o644)
}

func printInitNextSteps(project string) {
	fmt.Printf(`
✓ project '%s' is ready.

next steps:
  sdev use %s
  sdev new my-task
  sdev up my-task
  sdev open my-task
`, project, project)
}

func splitCommaSpace(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
}

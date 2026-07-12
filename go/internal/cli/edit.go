package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/paths"
	"gopkg.in/yaml.v3"
)

// Edit implements `sdev edit [<project>] [--delete-source]`: an interactive menu
// to add/remove repos and edit the conf prefix, shell service, and stack list.
// Mirrors bin/edit-project.
func Edit(args []string) int {
	deleteSource := false
	var positional []string
	for _, a := range args {
		if a == "--delete-source" {
			deleteSource = true
		} else {
			positional = append(positional, a)
		}
	}

	home := paths.Home()
	ensureHome(home)
	project := config.ResolveProject(home, "")
	if len(positional) > 0 && positional[0] != "" {
		project = positional[0]
	}
	if project == "" {
		return failMsg("no project given and none active")
	}
	projFile := filepath.Join(home, "core", "projects.d", project+".yml")
	if !fileExists(projFile) {
		return failMsg("project '" + project + "' not found - create it with: sdev init")
	}

	ed := &editor{home: home, project: project, projFile: projFile, deleteSource: deleteSource, p: newPrompter()}
	for {
		ed.show()
		fmt.Fprint(os.Stderr, "\n  [a] add repo   [r] remove repo   [p] conf prefix   [s] shell service   [t] stack services   [q] quit\n> ")
		choice := ed.p.readLine()
		switch choice {
		case "a":
			ed.addRepo()
		case "r":
			ed.removeRepo()
		case "p":
			ed.setScalar("conf_prefix", "Conf prefix")
		case "s":
			ed.setScalar("default_shell_service", "Shell service")
		case "t":
			ed.setStack()
		case "q", "":
			return 0
		default:
			fmt.Fprintf(os.Stderr, "  (not yet implemented: '%s')\n", choice)
		}
	}
}

// editor holds the interactive edit session state.
type editor struct {
	home, project, projFile string
	deleteSource            bool
	p                       *prompter
}

// projView is a project registry read directly (no global fallback), for the
// edit summary and existence checks.
type projView struct {
	ConfPrefix          string                    `yaml:"conf_prefix"`
	DefaultShellService string                    `yaml:"default_shell_service"`
	StackServices       []string                  `yaml:"stack_services"`
	Repos               map[string]map[string]any `yaml:"repos"`
}

func (e *editor) view() projView {
	var v projView
	if data, err := os.ReadFile(e.projFile); err == nil {
		_ = yaml.Unmarshal(data, &v)
	}
	return v
}

func (e *editor) show() {
	doc := e.view()
	fmt.Println()
	fmt.Printf("Project: %s  (%s)\n", e.project, e.projFile)
	fmt.Printf("  conf_prefix:    %s\n", orDefault(doc.ConfPrefix, "app"))
	fmt.Printf("  shell_service:  %s\n", orDefault(doc.DefaultShellService, "api"))
	if len(doc.StackServices) > 0 {
		fmt.Printf("  stack_services: %s\n", strings.Join(doc.StackServices, ", "))
	} else {
		fmt.Println("  stack_services: (inherits global default)")
	}
	fmt.Println("  repos:")
	for _, r := range sortedRepoKeys(doc.Repos) {
		path := config.RepoAttr(e.home, e.project, r, "path")
		base := config.RepoAttr(e.home, e.project, r, "default_base")
		role := config.RepoAttr(e.home, e.project, r, "compose_role")
		fmt.Printf("    %-12s base=%-8s role=%-8s %s\n", r, base, role, sourceKind(filepath.Join(e.home, "core", e.project, path)))
	}
}

func (e *editor) addRepo() {
	name := e.p.ask("Repo name", "")
	if !validSlug(name) {
		fmt.Fprintf(os.Stderr, "  invalid repo name '%s'\n", name)
		return
	}
	if e.repoExists(name) {
		fmt.Fprintf(os.Stderr, "  repo '%s' already exists\n", name)
		return
	}
	source := e.p.ask("  git URL to clone, or path to an existing local repo", "")
	base := e.p.ask("  default base branch", "main")
	if !validRef(base) {
		fmt.Fprintf(os.Stderr, "  invalid base branch '%s'\n", base)
		return
	}
	role := e.p.ask("  compose role", name)
	if !validToken(role) {
		fmt.Fprintf(os.Stderr, "  invalid compose role '%s'\n", role)
		return
	}
	verb, err := addRepoSource(e.home, e.project, name, source)
	if err != nil {
		fmt.Fprintln(os.Stderr, "  could not add source; aborting add")
		return
	}
	logf("%s repo '%s'", verb, name)
	e.mutate(func(root *yaml.Node) {
		repos := ensureMap(root, "repos")
		appendMapEntry(repos, name, repoEntryNode(name, base, role))
	})
	logf("added repo '%s'", name)
}

func (e *editor) removeRepo() {
	name := e.p.ask("Repo to remove", "")
	if !validToken(name) {
		fmt.Fprintf(os.Stderr, "  invalid repo name '%s'\n", name)
		return
	}
	if !e.repoExists(name) {
		fmt.Fprintf(os.Stderr, "  no such repo '%s'\n", name)
		return
	}
	path := config.RepoAttr(e.home, e.project, name, "path")
	if !e.clearWorktrees(name, path) {
		return
	}
	e.clearSource(filepath.Join(e.home, "core", e.project, path))
	e.mutate(func(root *yaml.Node) {
		if repos := mapValue(root, "repos"); repos != nil {
			deleteMapKey(repos, name)
		}
	})
	logf("removed repo '%s'", name)
}

// clearWorktrees removes live task worktrees of a repo after a 'force' confirm.
// Returns false (abort) when worktrees exist and the user did not type force.
func (e *editor) clearWorktrees(name, path string) bool {
	var wts []string
	dirs, _ := os.ReadDir(filepath.Join(e.home, "projects", e.project))
	for _, d := range dirs {
		wt := filepath.Join(e.home, "projects", e.project, d.Name(), path)
		if exists(filepath.Join(wt, ".git")) {
			wts = append(wts, wt)
		}
	}
	if len(wts) == 0 {
		return true
	}
	fmt.Fprintf(os.Stderr, "  ⚠ live task worktrees use '%s':\n", name)
	for _, w := range wts {
		fmt.Fprintf(os.Stderr, "      %s\n", w)
	}
	fmt.Fprintln(os.Stderr, "  end those tasks (sdev end <slug>), or type 'force' to remove the worktrees (branches kept):")
	if e.p.readLine() != "force" {
		fmt.Fprintf(os.Stderr, "  aborted removal of '%s'\n", name)
		return false
	}
	src := config.RepoSourceDir(e.home, e.project, path)
	for _, w := range wts {
		if runGit(src, "worktree", "remove", "--force", w) == nil {
			fmt.Fprintf(os.Stderr, "    removed worktree %s (branch kept)\n", w)
		} else {
			fmt.Fprintf(os.Stderr, "    ⚠ could not remove worktree %s - remove it manually\n", w)
		}
	}
	return true
}

// clearSource unlinks a symlinked source, or (with --delete-source) removes a
// clean cloned source, keeping it when it has uncommitted/unpushed work.
func (e *editor) clearSource(dest string) {
	info, err := os.Lstat(dest)
	if err != nil {
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(dest)
		logf("unlinked symlinked source %s", dest)
		return
	}
	if !info.IsDir() {
		return
	}
	if !e.deleteSource {
		logf("kept clone at %s (pass --delete-source to remove)", dest)
		return
	}
	if n := uncommittedCount(dest); n != 0 {
		fmt.Fprintf(os.Stderr, "  ⚠ %s has %d uncommitted/unpushed change(s). Type 'yes' to delete anyway:\n", dest, n)
		if e.p.readLine() != "yes" {
			fmt.Fprintf(os.Stderr, "  keeping source %s\n", dest)
			return
		}
	}
	_ = os.RemoveAll(dest)
	logf("deleted clone %s", dest)
}

func (e *editor) setScalar(key, label string) {
	v := e.p.ask(label, "")
	if !validToken(v) {
		fmt.Fprintf(os.Stderr, "  invalid value '%s'\n", v)
		return
	}
	e.mutate(func(root *yaml.Node) { setMapScalar(root, key, v) })
	logf("set %s = %s", key, v)
}

func (e *editor) setStack() {
	line := e.p.ask("Stack services (comma/space separated; blank = inherit global)", "")
	if line == "" {
		e.mutate(func(root *yaml.Node) { deleteMapKey(root, "stack_services") })
		logf("stack_services -> inherit global")
		return
	}
	var svcs []string
	for _, s := range splitCommaSpace(line) {
		if !validToken(s) {
			fmt.Fprintf(os.Stderr, "  invalid service '%s'\n", s)
			return
		}
		svcs = append(svcs, s)
	}
	e.mutate(func(root *yaml.Node) { setMapSequence(root, "stack_services", svcs) })
	logf("stack_services = %s", strings.Join(svcs, " "))
}

func (e *editor) repoExists(name string) bool {
	_, ok := e.view().Repos[name]
	return ok
}

// mutate loads the project file, applies fn to its root mapping, and writes it back.
func (e *editor) mutate(fn func(root *yaml.Node)) {
	data, err := os.ReadFile(e.projFile)
	if err != nil {
		return
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return
	}
	if len(doc.Content) != 1 {
		return
	}
	fn(doc.Content[0])
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return
	}
	_ = os.WriteFile(e.projFile, out, 0o644)
}

func uncommittedCount(dir string) int {
	total := 0
	if out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
			if line != "" {
				total++
			}
		}
	}
	if out, err := exec.Command("git", "-C", dir, "rev-list", "--count", "@{u}..HEAD").Output(); err == nil {
		if n, e := strconv.Atoi(strings.TrimSpace(string(out))); e == nil {
			total += n
		}
	}
	return total
}

func sourceKind(dest string) string {
	info, err := os.Lstat(dest)
	if err != nil {
		return "MISSING SOURCE"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(dest)
		return "symlink -> " + target
	}
	if info.IsDir() {
		return "clone"
	}
	return "MISSING SOURCE"
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func sortedRepoKeys(repos map[string]map[string]any) []string {
	keys := make([]string, 0, len(repos))
	for k := range repos {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

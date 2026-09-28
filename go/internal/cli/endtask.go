package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/infra"
	"github.com/santhosh2011/sdev/internal/lock"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/state"
)

// archiveItems are the task-dir entries preserved into the soft archive.
var archiveItems = []string{"knowledge", "CLAUDE.md", ".claude", ".env", "docker-compose.yml", "compose", "nginx.conf"}

// endArgs is the parsed `sdev end` command line.
type endArgs struct {
	slug        string
	force       bool
	keepBranch  bool
	mergeTarget string
	workBranch  string
	pool        bool
}

// taskRepo is one repo worktree in a task: its config name and worktree-relative path.
type taskRepo struct{ name, path string }

// End implements `sdev end <slug>`: tear a task down - stop docker, return each
// worktree to the warm pool (--pool) or remove it, delete branches, free the
// offset, and soft-archive knowledge. Mirrors bin/end-task.
func End(args []string) int {
	a, code, handled := parseEndArgs(args)
	if handled {
		return code
	}

	home := paths.Home()
	project := config.ActiveProject()
	taskDir, ok := resolveEndDir(home, project, a.slug)
	if !ok {
		return failMsg(fmt.Sprintf("task '%s' not found in project '%s'", a.slug, project))
	}
	taskKey := strings.TrimPrefix(taskDir, filepath.Join(home, "projects")+string(filepath.Separator))

	// An ephemeral task is torn down fully - never pooled, no cached deps kept.
	if taskIsEphemeral(home, taskKey) {
		if a.pool {
			logf("task is ephemeral - ignoring --pool (torn down fully, no cached deps kept)")
		}
		a.pool = false
	}

	if !a.force {
		if code := endPreflight(home, project, taskDir, a); code != 0 {
			return code
		}
	}

	// Tear down docker (best-effort; no-op without an executable ./compose).
	teardownOps().Down(taskDir)
	if err := infra.Drop(home, taskKey); err != nil {
		return failErr(err)
	}

	repos := collectTaskRepos(home, project, taskDir)
	finalSHAs := finalSHAs(taskDir, repos)
	pooled := freeWorktrees(home, project, a, taskDir, repos)

	if err := lock.With(state.Dir(home), func() error { return state.FreeTask(home, taskKey) }); err != nil {
		logf("warning: could not update state ledger for %s", taskKey)
	}

	archive := filepath.Join(home, "projects", "_archive", project, a.slug)
	if err := archiveTask(taskDir, archive, a, repos, finalSHAs, pooled); err != nil {
		return failErr(err)
	}
	os.RemoveAll(taskDir)
	logf("archived to projects/_archive/%s/%s", project, a.slug)
	return 0
}

func parseEndArgs(args []string) (endArgs, int, bool) {
	a := endArgs{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Println("Usage: end-task <slug> [--force] [--keep-branch] [--merge-target <branch>] [--work-branch <branch>] [--pool]")
			fmt.Println("Checks checked-out HEAD (including detached HEAD); --work-branch also checks a named local branch. Both must be landed.")
			fmt.Println("Target defaults to each repo's configured default_base; accepts a bare branch (on origin) or remote/branch.")
			fmt.Println("Fetches the target before checking ancestry; fetch failure refuses teardown. Squash/rebase merges may require separate review of rewritten commits.")
			return a, 0, true
		case "--force":
			a.force = true
		case "--keep-branch":
			a.keepBranch = true
		case "--merge-target", "--work-branch":
			if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "--") {
				return a, failMsg(args[i] + " requires a value"), true
			}
			if args[i] == "--merge-target" {
				a.mergeTarget = args[i+1]
			} else {
				a.workBranch = args[i+1]
			}
			i++
		case "--pool":
			a.pool = true
		default:
			if a.slug != "" {
				return a, failMsg("unexpected arg: " + args[i]), true
			}
			a.slug = args[i]
		}
	}
	if a.slug == "" {
		return a, failMsg("slug required"), true
	}
	return a, 0, false
}

// resolveEndDir resolves a task's workspace dir: namespaced, else legacy flat.
func resolveEndDir(home, project, slug string) (string, bool) {
	ns := filepath.Join(home, "projects", project, slug)
	if fsutil.IsDir(ns) {
		return ns, true
	}
	flat := filepath.Join(home, "projects", slug)
	if fsutil.IsDir(flat) {
		return flat, true
	}
	return "", false
}

// endPreflight enforces the non-force safety gate: every repo branch merged into
// the merge target, every worktree clean, and no containers still running.
func endPreflight(home, project, taskDir string, a endArgs) int {
	names, err := config.Repos(home, project)
	if err != nil {
		return failErr(err)
	}
	if len(names) == 0 {
		return failMsg("no project repos configured - refusing teardown")
	}
	for _, name := range names {
		path, err := config.RepoPath(home, project, name)
		if err != nil {
			return failErr(err)
		}
		repo := filepath.Join(taskDir, path)
		if !hasGitRepo(repo) {
			continue
		}
		target := a.mergeTarget
		if target == "" {
			target = config.RepoBase(home, project, name)
		}
		if err := checkEndLanded(repo, target, a.workBranch); err != nil {
			return failMsg(fmt.Sprintf("%s: %v", name, err))
		}
		if out, _ := exec.Command("git", "-C", repo, "status", "--porcelain").Output(); len(strings.TrimSpace(string(out))) > 0 {
			return failMsg(fmt.Sprintf("%s: working tree dirty - pass --force to override", name))
		}
	}
	cpn := readEnvValue(taskDir, "COMPOSE_PROJECT_NAME")
	if cpn == "" {
		cpn = a.slug
	}
	if out, _ := exec.Command("docker", "ps", "--filter", "label=com.docker.compose.project="+cpn, "-q").Output(); len(strings.TrimSpace(string(out))) > 0 {
		return failMsg(fmt.Sprintf("containers still running for project %s - stop them or pass --force", cpn))
	}
	return 0
}

// checkEndLanded checks the worktree's actual tip, never its setup branch.
// A named work branch is additional evidence, not permission to discard HEAD.
func checkEndLanded(repo, target, workBranch string) error {
	if target == "" || target == "null" {
		return fmt.Errorf("no default_base configured - specify --merge-target")
	}
	remote, branch := "origin", target
	if prefix, rest, ok := strings.Cut(target, "/"); ok {
		if exec.Command("git", "-C", repo, "remote", "get-url", prefix).Run() == nil {
			remote, branch = prefix, rest
		}
	}
	if exec.Command("git", "check-ref-format", "refs/heads/"+branch).Run() != nil {
		return fmt.Errorf("invalid merge target %q", target)
	}
	targetRef := "refs/remotes/" + remote + "/" + branch
	if err := runGit(repo, "fetch", "--quiet", "--no-tags", remote, "+refs/heads/"+branch+":"+targetRef); err != nil {
		return fmt.Errorf("cannot fetch %s/%s - refusing teardown", remote, branch)
	}
	refs := []string{"HEAD"}
	if workBranch != "" {
		refs = append(refs, "refs/heads/"+workBranch)
	}
	for _, ref := range refs {
		label := ref
		if ref == "HEAD" {
			name, err := exec.Command("git", "-C", repo, "symbolic-ref", "--quiet", "--short", "HEAD").Output()
			label = "HEAD (detached)"
			if err == nil {
				label = "HEAD (" + strings.TrimSpace(string(name)) + ")"
			}
		}
		if exec.Command("git", "-C", repo, "merge-base", "--is-ancestor", ref, targetRef).Run() != nil {
			return fmt.Errorf("%s not in %s/%s - pass --force to override", label, remote, branch)
		}
	}
	return nil
}

// collectTaskRepos returns the project repos that have a worktree in the task dir.
func collectTaskRepos(home, project, taskDir string) []taskRepo {
	names, _ := config.Repos(home, project)
	var repos []taskRepo
	for _, name := range names {
		p, _ := config.RepoPath(home, project, name)
		if fsutil.IsDir(filepath.Join(taskDir, p)) {
			repos = append(repos, taskRepo{name: name, path: p})
		}
	}
	return repos
}

func finalSHAs(taskDir string, repos []taskRepo) map[string]string {
	shas := map[string]string{}
	for _, r := range repos {
		out, err := exec.Command("git", "-C", filepath.Join(taskDir, r.path), "rev-parse", "HEAD").Output()
		if err != nil {
			shas[r.path] = "unknown"
			continue
		}
		shas[r.path] = strings.TrimSpace(string(out))
	}
	return shas
}

// freeWorktrees returns each worktree to the pool (--pool) or removes it, then
// deletes the task branch unless --keep-branch. Returns the pooled repo paths.
func freeWorktrees(home, project string, a endArgs, taskDir string, repos []taskRepo) []string {
	var pooled []string
	for _, r := range repos {
		src := config.RepoSourceDir(home, project, r.path)
		wt := filepath.Join(taskDir, r.path)
		returned := false
		if a.pool {
			returned = returnToPool(home, project, r, src, wt)
			if returned {
				pooled = append(pooled, r.path)
			}
		}
		if !returned {
			_ = runGit(src, "worktree", "remove", "--force", wt)
		}
		if !a.keepBranch {
			_ = runGit(src, "branch", "-D", "task/"+a.slug)
		}
	}
	return pooled
}

// returnToPool resets a worktree clean (keeping gitignored caches), detaches it,
// and relocates it into the warm pool, recording the entry. Any hiccup logs a
// warning, cleans up the reserved slot, and returns false so the caller removes
// the worktree instead.
func returnToPool(home, project string, r taskRepo, src, wt string) bool {
	var dest string
	if err := lock.With(state.Dir(home), func() error {
		d, e := state.ReservePoolSlot(home, project, r.path)
		dest = d
		return e
	}); err != nil {
		return false
	}
	_ = os.MkdirAll(filepath.Dir(dest), 0o755)

	if runGit(wt, "reset", "--hard") == nil &&
		runGit(wt, "clean", "-fd") == nil &&
		runGit(wt, "checkout", "--detach") == nil &&
		runGit(src, "worktree", "move", wt, dest) == nil {
		_ = lock.With(state.Dir(home), func() error {
			return state.RecordPool(home, state.PoolEntry{Project: project, Repo: r.name, RepoPath: r.path, Source: src, Path: dest})
		})
		logf("returned %s to pool: %s", r.path, dest)
		return true
	}
	logf("warning: could not pool %s - removing instead", r.path)
	os.RemoveAll(dest)
	_ = lock.With(state.Dir(home), func() error { return state.DropPool(home, dest) })
	return false
}

// archiveTask soft-archives the task's knowledge and config, writing ARCHIVE_INFO.md.
func archiveTask(taskDir, archive string, a endArgs, repos []taskRepo, finalSHAs map[string]string, pooled []string) error {
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return err
	}
	for _, item := range archiveItems {
		if exists(filepath.Join(taskDir, item)) {
			_ = os.Rename(filepath.Join(taskDir, item), filepath.Join(archive, item))
		}
	}
	offset := readEnvValue(archive, "PORT_OFFSET")
	if offset == "" {
		offset = "unknown"
	}
	return os.WriteFile(filepath.Join(archive, "ARCHIVE_INFO.md"), []byte(archiveInfo(a, offset, repos, finalSHAs, pooled)), 0o644)
}

func archiveInfo(a endArgs, offset string, repos []taskRepo, finalSHAs map[string]string, pooled []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Archive: %s\n\n", a.slug)
	fmt.Fprintf(&b, "- archive_date: %s\n", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	target := a.mergeTarget
	if target == "" {
		target = "per-repo default_base"
	}
	fmt.Fprintf(&b, "- merge_target: %s\n", target)
	fmt.Fprintf(&b, "- port_offset_freed: %s\n", offset)
	if len(pooled) > 0 {
		fmt.Fprintf(&b, "- pooled_worktrees: %s\n", strings.Join(pooled, " "))
	}
	b.WriteString("\n## Final commits\n")
	for _, r := range repos {
		fmt.Fprintf(&b, "- %s: %s (branch task/%s)\n", r.path, finalSHAs[r.path], a.slug)
	}
	return b.String()
}

func taskIsEphemeral(home, key string) bool {
	l, err := state.Load(state.FilePath(home))
	if err != nil {
		return false
	}
	return l.Tasks[key].Ephemeral
}

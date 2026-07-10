package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/lock"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/proc"
	"github.com/santhosh2011/sdev/internal/state"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// newArgs is the parsed `sdev new` command line.
type newArgs struct {
	slug         string
	repos        string // raw --repos csv; empty means all
	envProfile   string
	fetch        bool
	usePool      bool
	lease        bool
	leaseHolder  string
	ephemeral    bool
	baseOverride map[string]string
}

// Newtask implements `sdev new <slug> [flags]`: create an isolated task workspace
// (git worktree(s) on task/<slug> + a reserved port offset + generated compose,
// .env, app.env, CLAUDE.md, and Claude settings). Mirrors bin/new-task.
func Newtask(args []string) int {
	a, code, handled := parseNewArgs(args)
	if handled {
		return code
	}

	home := paths.Home()
	project := config.ActiveProject()
	taskDir := filepath.Join(home, "projects", project, a.slug)
	if exists(taskDir) {
		return failMsg(fmt.Sprintf("projects/%s/%s already exists", project, a.slug))
	}
	if exists(filepath.Join(home, "projects", "_archive", project, a.slug)) {
		return failMsg(fmt.Sprintf("archived task projects/_archive/%s/%s exists - restore or pick another slug", project, a.slug))
	}

	profile, err := config.ResolveProfileForNew(home, a.envProfile)
	if err != nil {
		return failErr(err)
	}
	logf("slug=%s", a.slug)
	logf("env profile=%s", profile)

	selected, code := selectRepos(home, project, a.repos)
	if code != 0 {
		return code
	}

	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return failErr(err)
	}

	created := []string{}
	for _, repo := range selected {
		if err := createWorktree(home, project, a, repo, taskDir); err != nil {
			rollbackNew(home, project, a.slug, taskDir, created)
			return failErr(err)
		}
		created = append(created, repo)
	}
	logf("worktrees created: %s", strings.Join(created, " "))

	offset, err := reserveOffset(home, project, a)
	if err != nil {
		return failErr(err)
	}
	logf("port offset=%d", offset)
	if a.lease {
		suffix := ""
		if a.leaseHolder != "" {
			suffix = " to " + a.leaseHolder
		}
		logf("leased%s", suffix)
	}
	if a.ephemeral {
		logf("ephemeral (auto-reclaimable by 'sdev prune'; never pooled)")
	}

	if err := writeTaskFiles(home, project, a.slug, profile, offset, selected, taskDir); err != nil {
		return failErr(err)
	}
	printNextSteps(project, a.slug, offset, taskDir)
	return 0
}

// parseNewArgs parses the command line. The bool return is true when the caller
// should return immediately with the given exit code (help or a parse error).
func parseNewArgs(args []string) (newArgs, int, bool) {
	a := newArgs{fetch: true, usePool: true, baseOverride: map[string]string{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			newPrintUsage()
			return a, 0, true
		case arg == "--repos":
			if i+1 >= len(args) {
				return a, failMsg("--repos requires a value"), true
			}
			a.repos = args[i+1]
			i++
		case arg == "--env":
			if i+1 >= len(args) {
				return a, failMsg("--env requires a value"), true
			}
			a.envProfile = args[i+1]
			i++
		case arg == "--no-fetch":
			a.fetch = false
		case arg == "--no-pool":
			a.usePool = false
		case arg == "--lease":
			a.lease = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				a.leaseHolder = args[i+1]
				i++
			}
		case arg == "--ephemeral":
			a.ephemeral = true
		case strings.HasPrefix(arg, "--") && strings.HasSuffix(arg, "-base"):
			key := strings.TrimSuffix(strings.TrimPrefix(arg, "--"), "-base")
			if i+1 >= len(args) {
				return a, failMsg("--" + key + "-base requires a value"), true
			}
			a.baseOverride[key] = args[i+1]
			i++
		case strings.HasPrefix(arg, "--"):
			return a, failMsg("unknown flag: " + arg), true
		default:
			if a.slug != "" {
				return a, failMsg("unexpected arg: " + arg), true
			}
			a.slug = arg
		}
	}
	if a.leaseHolder == "" {
		a.leaseHolder = os.Getenv("SDEV_LEASE_HOLDER")
	}
	if a.lease && a.ephemeral {
		return a, failMsg("--lease and --ephemeral are mutually exclusive (a lease is durable; ephemeral is auto-reclaimable)"), true
	}
	if a.slug == "" {
		return a, failMsg("slug required"), true
	}
	if !validSlug(a.slug) {
		return a, failMsg("invalid slug '" + a.slug + "' - must be kebab-case, 1-50 chars"), true
	}
	return a, 0, false
}

// selectRepos resolves the repos to include: the --repos csv (validated against
// the project's repos) or all of them.
func selectRepos(home, project, reposArg string) ([]string, int) {
	all, err := config.Repos(home, project)
	if err != nil {
		return nil, failErr(err)
	}
	if reposArg == "" {
		return all, 0
	}
	selected := strings.Split(reposArg, ",")
	for _, r := range selected {
		if !contains(all, r) {
			return nil, failMsg(fmt.Sprintf("unknown repo '%s' (allowed: %s)", r, strings.Join(all, " ")))
		}
	}
	return selected, 0
}

// createWorktree materializes one repo's task worktree: fetch the base, pick the
// freshest start point, and either reuse a warm-pool worktree (re-branded) or add
// a fresh one.
func createWorktree(home, project string, a newArgs, repo, taskDir string) error {
	repoPath, err := config.RepoPath(home, project, repo)
	if err != nil {
		return err
	}
	base := a.baseOverride[repo]
	if base == "" {
		base = config.RepoBase(home, project, repo)
	}
	src := config.RepoSourceDir(home, project, repoPath)
	if !hasGitRepo(src) {
		return fmt.Errorf("%s: no git source at %s", repoPath, src)
	}

	if a.fetch {
		if runGit(src, "fetch", "--quiet", "origin", base) == nil {
			logf("fetched origin/%s for %s", base, repoPath)
		} else {
			logf("warning: %s: fetch of '%s' from origin failed - base may be stale", repoPath, base)
		}
	}

	startPoint := ""
	switch {
	case gitRefExists(src, "refs/remotes/origin/"+base):
		startPoint = "origin/" + base
	case gitRefExists(src, "refs/heads/"+base):
		startPoint = base
	default:
		return fmt.Errorf("%s: base branch '%s' not found locally or on origin", repoPath, base)
	}

	worktree := filepath.Join(taskDir, repoPath)
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		return err
	}

	if a.usePool && reusePooled(home, src, worktree, a.slug, startPoint, repo, repoPath) {
		return nil
	}

	logf("creating worktree %s (%s) on branch task/%s from %s", repo, repoPath, a.slug, startPoint)
	if err := runGit(src, "worktree", "add", "--no-track", worktree, "-b", "task/"+a.slug, startPoint); err != nil {
		return fmt.Errorf("git worktree add failed for %s: %w", repoPath, err)
	}
	return nil
}

// reusePooled tries to reuse a warm-pool worktree for src, re-branding it to
// task/<slug>. It returns true on success; on any hiccup it discards the stale
// slot and returns false so the caller creates a fresh worktree.
func reusePooled(home, src, worktree, slug, startPoint, repo, repoPath string) bool {
	var pooled string
	if err := lock.With(state.Dir(home), func() error {
		p, e := state.TakePool(home, src)
		pooled = p
		return e
	}); err != nil || pooled == "" {
		return false
	}
	if fsutil.IsDir(pooled) &&
		runGit(src, "worktree", "move", pooled, worktree) == nil &&
		runGit(worktree, "checkout", "--no-track", "-B", "task/"+slug, startPoint) == nil {
		logf("reused pooled worktree for %s (%s) -> task/%s", repo, repoPath, slug)
		return true
	}
	_ = runGit(src, "worktree", "remove", "--force", worktree)
	os.RemoveAll(pooled)
	os.RemoveAll(worktree)
	logf("warning: pooled reuse failed for %s - creating fresh", repo)
	return false
}

// rollbackNew undoes the worktrees created so far when a later one fails, then
// removes the task dir. Mirrors new-task's ERR-trap rollback.
func rollbackNew(home, project, slug, taskDir string, created []string) {
	for _, repo := range created {
		rp, _ := config.RepoPath(home, project, repo)
		src := config.RepoSourceDir(home, project, rp)
		_ = runGit(src, "worktree", "remove", "--force", filepath.Join(taskDir, rp))
		_ = runGit(src, "branch", "-D", "task/"+slug)
	}
	os.RemoveAll(taskDir)
	logf("rolled back")
}

// reserveOffset reserves a port offset for the task under the state lock.
func reserveOffset(home, project string, a newArgs) (int, error) {
	res := state.Reservation{Key: project + "/" + a.slug, Lease: a.lease, Holder: a.leaseHolder, Ephemeral: a.ephemeral}
	offset := 0
	err := lock.With(state.Dir(home), func() error {
		o, e := state.AllocateOffset(home, res, config.PortStep(home), proc.Alive)
		offset = o
		return e
	})
	return offset, err
}

func validSlug(slug string) bool {
	return len(slug) <= 50 && slugPattern.MatchString(slug)
}

func runGit(dir string, args ...string) error {
	return exec.Command("git", append([]string{"-C", dir}, args...)...).Run()
}

func gitRefExists(dir, ref string) bool {
	return runGit(dir, "show-ref", "--verify", "--quiet", ref) == nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func newPrintUsage() {
	fmt.Println(`Usage: new-task <slug> [--env local|dev|staging] [--repos api,ui] [--api-base qa] [--ui-base qa]
                       [--no-fetch] [--no-pool] [--lease [holder]] [--ephemeral]

Creates an isolated parallel-task workspace under projects/<project>/<slug>/.`)
}

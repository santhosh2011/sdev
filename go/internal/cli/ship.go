package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/paths"
)

// shipResult is the --json payload of `sdev ship`.
type shipResult struct {
	Task     string   `json:"task"`
	Project  string   `json:"project"`
	Assignee string   `json:"assignee"`
	Pushed   []string `json:"pushed"`
	PRs      []prRef  `json:"prs"`
	GH       bool     `json:"gh"`
	Next     []string `json:"next"`
}

type prRef struct {
	Repo string `json:"repo"`
	URL  string `json:"url"`
}

// Ship implements `sdev ship <slug> [--assignee <who>] [--force] [--json]`: push
// each repo's task branch and open/update a PR with the assignee set. Merge stays
// a human decision. Mirrors bin/ship.
func Ship(args []string) int {
	jsonOut, force, assignee, slug := false, false, "@me", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOut = true
		case "--force", "-f":
			force = true
		case "--assignee":
			if i+1 >= len(args) {
				return failMsg("--assignee requires a value")
			}
			assignee = args[i+1]
			i++
		default:
			if slug != "" {
				return failMsg("unexpected arg: " + args[i])
			}
			slug = args[i]
		}
	}
	if slug == "" {
		return failMsg("slug required (usage: sdev ship <slug> [--assignee <who>] [--force] [--json])")
	}

	home, project := paths.Home(), config.ActiveProject()
	taskDir, taskKey, ok := resolveTaskKey(home, project, slug)
	if !ok {
		return failMsg(fmt.Sprintf("task '%s' not found in project '%s'", slug, project))
	}

	haveGH := commandExists("gh")
	var pushed []string
	var prs []prRef
	shipped := false
	repos, _ := config.Repos(home, project)
	for _, r := range repos {
		p, _ := config.RepoPath(home, project, r)
		wt := filepath.Join(taskDir, p)
		if !hasGitRepo(wt) {
			continue
		}
		shipped = true
		base := config.RepoBase(home, project, r)

		if !force && worktreeDirty(wt) {
			return failMsg(r + ": working tree dirty -- commit/stash first, or pass --force")
		}
		push := exec.Command("git", "-C", wt, "push", "-u", "origin", "task/"+slug)
		push.Stdout, push.Stderr = os.Stderr, os.Stderr
		if err := push.Run(); err != nil {
			return failMsg(r + ": git push failed for task/" + slug)
		}
		pushed = append(pushed, r)

		url := ""
		if haveGH {
			out, _ := runInDir(wt, "gh", "pr", "create", "--base", base, "--head", "task/"+slug, "--assignee", assignee, "--fill")
			url = lastLine(out)
		}
		prs = append(prs, prRef{Repo: r, URL: url})
	}
	if !shipped {
		return failMsg("no repo worktrees found under " + taskDir)
	}

	if jsonOut {
		data, _ := json.MarshalIndent(shipResult{
			Task: taskKey, Project: project, Assignee: assignee,
			Pushed: nonNil(pushed), PRs: prs, GH: haveGH,
			Next: []string{"merge is yours: gh pr merge"},
		}, "", "  ")
		fmt.Println(string(data))
		return 0
	}
	fmt.Printf("sdev ship: %s (assignee: %s)\n", taskKey, assignee)
	if haveGH {
		for _, pr := range prs {
			fmt.Printf("  %s: %s\n", pr.Repo, pr.URL)
		}
	} else {
		fmt.Printf("  pushed %s -- gh not found, open the PR manually\n", strings.Join(pushed, ", "))
	}
	fmt.Println("  Next: merge is yours (gh pr merge)")
	return 0
}

// resolveTaskKey resolves a slug to its task dir and ledger key (namespaced then
// legacy flat).
func resolveTaskKey(home, project, slug string) (dir, key string, ok bool) {
	ns := filepath.Join(home, "projects", project, slug)
	if dirExists(ns) {
		return ns, project + "/" + slug, true
	}
	flat := filepath.Join(home, "projects", slug)
	if dirExists(flat) {
		return flat, slug, true
	}
	return "", "", false
}

func worktreeDirty(wt string) bool {
	out, err := exec.Command("git", "-C", wt, "status", "--porcelain").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func runInDir(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	last := lines[len(lines)-1]
	return strings.TrimSpace(last)
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

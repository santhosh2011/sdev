package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/paths"
)

var urlPattern = regexp.MustCompile(`https?://[^ ]+`)

type reviewResult struct {
	Task      string       `json:"task"`
	Project   string       `json:"project"`
	Repos     []reviewRepo `json:"repos"`
	Artifact  string       `json:"artifact"`
	URL       string       `json:"url"`
	LavishURL string       `json:"lavish_url"`
	Gate      gateResult   `json:"gate"`
	Next      []string     `json:"next"`
}

type reviewRepo struct {
	Repo    string `json:"repo"`
	Files   int    `json:"files"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

type gateResult struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Review implements `sdev review <slug> [--no-open] [--no-gate] [--json]`: render
// each repo's diff into an annotatable lavish HTML surface and run the quality
// gate, whose exit code is the verdict. Mirrors bin/review.
func Review(args []string) int {
	jsonOut, noOpen, noGate, slug := false, false, false, ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOut = true
		case "--no-open":
			noOpen = true
		case "--no-gate":
			noGate = true
		default:
			if slug != "" {
				return failMsg("unexpected arg: " + args[i])
			}
			slug = args[i]
		}
	}
	if slug == "" {
		return failMsg("slug required (usage: sdev review <slug> [--no-open] [--json])")
	}

	home, project := paths.Home(), config.ActiveProject()
	taskDir, taskKey, ok := resolveTaskKey(home, project, slug)
	if !ok {
		return failMsg(fmt.Sprintf("task '%s' not found in project '%s'", slug, project))
	}

	repos, body, firstWt := collectDiffs(home, project, taskDir)
	if firstWt == "" {
		return failMsg("no repo worktrees found under " + taskDir)
	}

	port := readEnvValue(taskDir, "NGINX_HOST_PORT")
	url := ""
	if port != "" {
		url = "http://localhost:" + port + "/"
	}

	art := filepath.Join(taskDir, ".lavish", "review-"+slug+".html")
	if err := writeReviewArtifact(art, taskKey, url, body); err != nil {
		return failErr(err)
	}

	lavishURL := ""
	if !noOpen && commandExists("lavish-axi") {
		out, _ := exec.Command("lavish-axi", art).Output()
		lavishURL = urlPattern.FindString(string(out))
	}

	gate := runGate(noGate, firstWt)

	if jsonOut {
		data, _ := json.MarshalIndent(reviewResult{
			Task: taskKey, Project: project, Repos: repos, Artifact: art, URL: url,
			LavishURL: lavishURL, Gate: gate,
			Next: []string{"annotate the diff in lavish, then: sdev ship " + slug},
		}, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("sdev review: %s\n", taskKey)
		for _, r := range repos {
			fmt.Printf("  %s: %d files (+%d/-%d)\n", r.Repo, r.Files, r.Added, r.Removed)
		}
		if lavishURL != "" {
			fmt.Printf("  review surface: %s\n", lavishURL)
		} else {
			fmt.Printf("  artifact: %s\n", art)
		}
		fmt.Printf("  gate: %s", gate.Status)
		if gate.Detail != "" {
			fmt.Printf(" (%s)", gate.Detail)
		}
		fmt.Printf("\n  Next: annotate, then sdev ship %s\n", slug)
	}

	if gate.Status == "needs-decisions" {
		return 1
	}
	return 0
}

// collectDiffs walks the task's repo worktrees, tallying each diff's numstat and
// building the artifact body. Returns the per-repo summary, the HTML body, and
// the first worktree (where the gate runs).
func collectDiffs(home, project, taskDir string) ([]reviewRepo, string, string) {
	var repos []reviewRepo
	var body strings.Builder
	firstWt := ""
	names, _ := config.Repos(home, project)
	for _, r := range names {
		p, _ := config.RepoPath(home, project, r)
		wt := filepath.Join(taskDir, p)
		if !hasGitRepo(wt) {
			continue
		}
		if firstWt == "" {
			firstWt = wt
		}
		baseref := diffBaseRef(wt, config.RepoBase(home, project, r))
		files, added, removed := numstat(wt, baseref)
		repos = append(repos, reviewRepo{Repo: r, Files: files, Added: added, Removed: removed})

		diff, _ := exec.Command("git", "-C", wt, "diff", baseref+"...HEAD").Output()
		fmt.Fprintf(&body, "<h2 class=\"text-lg font-bold mt-6 mb-2\">%s <span class=\"opacity-60 text-sm\">(%d files, +%d/-%d)</span></h2>", r, files, added, removed)
		fmt.Fprintf(&body, "<pre class=\"diff bg-base-100 rounded-box p-3 border\">%s</pre>", htmlEscape(string(diff)))
	}
	return repos, body.String(), firstWt
}

// diffBaseRef prefers origin/<base> when it exists, else the local base.
func diffBaseRef(wt, base string) string {
	if gitRefExists(wt, "refs/remotes/origin/"+base) {
		return "origin/" + base
	}
	return base
}

// numstat sums a diff's files/added/removed, ignoring binary ("-") counts.
func numstat(wt, baseref string) (files, added, removed int) {
	out, _ := exec.Command("git", "-C", wt, "diff", "--numstat", baseref+"...HEAD").Output()
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		files++
		if len(fields) >= 2 {
			if a, err := strconv.Atoi(fields[0]); err == nil {
				added += a
			}
			if d, err := strconv.Atoi(fields[1]); err == nil {
				removed += d
			}
		}
	}
	return files, added, removed
}

// runGate runs the configured quality gate (SDEV_GATE_CMD, default no-mistakes)
// once in the first worktree; its exit code is the verdict. Degrades to skipped.
func runGate(noGate bool, firstWt string) gateResult {
	gateCmd := os.Getenv("SDEV_GATE_CMD")
	if gateCmd == "" {
		gateCmd = "no-mistakes"
	}
	if noGate {
		return gateResult{Status: "skipped", Detail: gateCmd + " not run"}
	}
	if !commandExists(gateCmd) {
		return gateResult{Status: "skipped", Detail: gateCmd + " not installed"}
	}
	cmd := exec.Command(gateCmd)
	cmd.Dir = firstWt
	out, err := cmd.CombinedOutput()
	status := "clean"
	if err != nil {
		status = "needs-decisions"
	}
	return gateResult{Status: status, Detail: firstNonEmptyLine(string(out))}
}

func writeReviewArtifact(art, taskKey, url, body string) error {
	if err := os.MkdirAll(filepath.Dir(art), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<!DOCTYPE html><html lang="en" data-theme="luxury"><head><meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0"><title>review: %s</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/daisyui@5.5.19/daisyui.css">
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/daisyui@5.5.19/themes.css">
<script src="https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4.2.4/dist/index.global.js"></script>
<style>.diff{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;white-space:pre-wrap;overflow-wrap:anywhere}</style>
</head><body class="bg-base-200 text-base-content"><div class="max-w-6xl mx-auto p-6">
<h1 class="text-2xl font-bold mb-1">review: %s</h1>
`, taskKey, taskKey)
	if url != "" {
		fmt.Fprintf(&b, "<p class=\"mb-2 text-sm\">live: <a class=\"link link-primary\" href=\"%s\">%s</a></p>\n", url, url)
	}
	b.WriteString(body)
	b.WriteString("\n</div></body></html>\n")
	return os.WriteFile(art, []byte(b.String()), 0o644)
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.ReplaceAll(s, ">", "&gt;")
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

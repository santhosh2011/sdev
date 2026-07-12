package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	tokenPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	refPattern   = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

// prompter reads interactive wizard answers from stdin, prompting on stderr so
// captured stdout stays clean. Mirrors the ask() helper in bin/_lib.sh.
type prompter struct{ r *bufio.Reader }

func newPrompter() *prompter { return &prompter{r: bufio.NewReader(os.Stdin)} }

// ask prints "label [def]: " (or "label: ") to stderr and returns the trimmed
// answer, or def on an empty line or EOF.
func (p *prompter) ask(label, def string) string {
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	line, _ := p.r.ReadString('\n')
	if ans := strings.TrimSpace(line); ans != "" {
		return ans
	}
	return def
}

// readLine reads one line (no prompt, no default), returning it trimmed, or ""
// on EOF. Used for the edit menu selector and the force/yes confirmations.
func (p *prompter) readLine() string {
	line, _ := p.r.ReadString('\n')
	return strings.TrimSpace(line)
}

func validToken(s string) bool { return tokenPattern.MatchString(s) }
func validRef(s string) bool   { return refPattern.MatchString(s) }

// addRepoSource wires a repo's source under core/<project>/<name>: cloning a git
// URL, or symlinking an existing local repo. Returns the verb ("cloned"/"linked").
// Mirrors add_repo_source in bin/_lib.sh.
func addRepoSource(home, project, name, spec string) (string, error) {
	spec = expandHome(spec)
	dest := filepath.Join(home, "core", project, name)
	if exists(dest) {
		return "", fmt.Errorf("source already exists at %s", dest)
	}
	if err := os.MkdirAll(filepath.Join(home, "core", project), 0o755); err != nil {
		return "", err
	}
	switch {
	case strings.Contains(spec, "://") || strings.HasPrefix(spec, "git@"):
		cmd := exec.Command("git", "clone", "-q", spec, dest)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("clone failed: %s", spec)
		}
		return "cloned", nil
	case hasGitRepo(spec):
		abs, _ := filepath.Abs(spec)
		if err := os.Symlink(abs, dest); err != nil {
			return "", err
		}
		return "linked", nil
	case exists(spec):
		return "", fmt.Errorf("'%s' exists but is not a git repo (no .git)", spec)
	default:
		return "", fmt.Errorf("'%s' is not a git URL or an existing local repo", spec)
	}
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~") {
		return filepath.Join(os.Getenv("HOME"), p[1:])
	}
	return p
}

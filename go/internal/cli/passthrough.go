package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/task"
)

// Up implements `sdev up <slug> [--env <profile>] [--yes] [extra...]`: optionally
// switch the task's env profile, enforce the staging guard, then boot the stack
// (compose up -d). Mirrors the up block in bin/sdev.
func Up(args []string) int {
	slug, profileFlag := "", ""
	assumeYes := false
	var extra []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--env":
			if i+1 >= len(args) {
				return failMsg("--env requires a value")
			}
			profileFlag = args[i+1]
			i++
		case "--yes", "-y":
			assumeYes = true
		default:
			if slug == "" {
				slug = args[i]
			} else {
				extra = append(extra, args[i])
			}
		}
	}
	home, project := paths.Home(), config.ActiveProject()
	dir, code := resolveTaskDir(home, project, slug)
	if code != 0 {
		return code
	}

	if profileFlag != "" {
		if !config.ValidProfile(profileFlag) {
			return failMsg(fmt.Sprintf("invalid env profile '%s' (allowed: %v)", profileFlag, config.ValidProfiles()))
		}
		conf := config.ProfileConfFile(home, profileFlag, project)
		link := filepath.Join(dir, "app.env")
		_ = os.Remove(link)
		_ = os.Symlink(conf, link)
		if err := setAppEnv(dir, profileFlag); err != nil {
			return failErr(err)
		}
	}

	profile := readEnvValue(dir, "APP_ENV")
	if profile == "" {
		profile = "local"
	}
	if profile == "staging" && !confirmStaging(assumeYes) {
		return failMsg("aborted (staging not confirmed)")
	}
	return execCompose(dir, append([]string{"up", "-d"}, extra...)...)
}

// Down implements `sdev down <slug> [extra...]`: stop the stack.
func Down(args []string) int {
	dir, extra, code := taskDirAndRest(args)
	if code != 0 {
		return code
	}
	return execCompose(dir, append([]string{"down"}, extra...)...)
}

// Nuke implements `sdev nuke <slug>`: stop the stack and reclaim its volumes.
func Nuke(args []string) int {
	dir, _, code := taskDirAndRest(args)
	if code != 0 {
		return code
	}
	return execCompose(dir, "down", "-v", "--remove-orphans")
}

// Logs implements `sdev logs <slug> [--no-follow] [services...]`.
func Logs(args []string) int {
	if len(args) == 0 {
		return failMsg("slug required")
	}
	home, project := paths.Home(), config.ActiveProject()
	dir, code := resolveTaskDir(home, project, args[0])
	if code != 0 {
		return code
	}
	follow := true
	var services []string
	for _, a := range args[1:] {
		if a == "--no-follow" {
			follow = false
		} else {
			services = append(services, a)
		}
	}
	logsArgs := []string{"logs"}
	if follow {
		logsArgs = append(logsArgs, "-f")
	}
	return execCompose(dir, append(logsArgs, services...)...)
}

// Shell implements `sdev shell <slug> [service]`: exec sh in a service.
func Shell(args []string) int {
	if len(args) == 0 {
		return failMsg("slug required")
	}
	home, project := paths.Home(), config.ActiveProject()
	dir, code := resolveTaskDir(home, project, args[0])
	if code != 0 {
		return code
	}
	svc := config.ShellService(home, project)
	if len(args) > 1 {
		svc = args[1]
	}
	return execCompose(dir, "exec", "-it", svc, "sh")
}

// Open implements `sdev open <slug>`: open the task's nginx URL in a browser.
func Open(args []string) int {
	dir, _, code := taskDirAndRest(args)
	if code != 0 {
		return code
	}
	if !fileExists(filepath.Join(dir, ".env")) {
		return failMsg(filepath.Join(dir, ".env") + " not found")
	}
	port := readEnvValue(dir, "NGINX_HOST_PORT")
	if port == "" {
		return failMsg("NGINX_HOST_PORT not set in " + filepath.Join(dir, ".env"))
	}
	url := "http://localhost:" + port + "/"
	if commandExists("open") {
		_ = exec.Command("open", url).Run()
	} else {
		fmt.Println(url)
	}
	return 0
}

// Code implements `sdev code <slug>`: open the task dir in an editor.
func Code(args []string) int {
	dir, _, code := taskDirAndRest(args)
	if code != 0 {
		return code
	}
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if ed := os.Getenv(env); ed != "" {
			return execEditor(ed, dir)
		}
	}
	for _, ed := range []string{"zed", "code", "cursor"} {
		if commandExists(ed) {
			return execEditor(ed, dir)
		}
	}
	fmt.Fprintf(os.Stderr, "No editor found. Set $EDITOR/$VISUAL, or open: %s\n", dir)
	fmt.Println(dir)
	return 0
}

// Cd implements `sdev cd <slug>`: print the absolute task dir.
func Cd(args []string) int {
	dir, _, code := taskDirAndRest(args)
	if code != 0 {
		return code
	}
	fmt.Println(dir)
	return 0
}

// taskDirAndRest resolves the first arg as a slug and returns the task dir plus
// the remaining args.
func taskDirAndRest(args []string) (string, []string, int) {
	slug := ""
	if len(args) > 0 {
		slug = args[0]
	}
	dir, code := resolveTaskDir(paths.Home(), config.ActiveProject(), slug)
	if code != 0 {
		return "", nil, code
	}
	return dir, args[1:], 0
}

func resolveTaskDir(home, project, slug string) (string, int) {
	dir, err := task.Resolve(home, project, slug)
	if err != nil {
		return "", failErr(err)
	}
	return dir, 0
}

// execCompose chdirs into the task dir and replaces this process with its
// ./compose wrapper, matching the bash `cd "$dir"; exec ./compose ...` so the
// stack inherits the terminal, signals, and exit status directly.
func execCompose(dir string, args ...string) int {
	compose := filepath.Join(dir, "compose")
	if err := os.Chdir(dir); err != nil {
		return failErr(err)
	}
	if err := syscall.Exec(compose, append([]string{compose}, args...), os.Environ()); err != nil {
		return failErr(err)
	}
	return 0 // unreachable: Exec replaces the process on success
}

func execEditor(editor, dir string) int {
	// $VISUAL/$EDITOR may carry args (e.g. "code -w"); split like the shell's exec.
	fields := strings.Fields(editor)
	bin, err := exec.LookPath(fields[0])
	if err != nil {
		return failErr(err)
	}
	argv := append(fields, dir)
	if err := syscall.Exec(bin, argv, os.Environ()); err != nil {
		return failErr(err)
	}
	return 0
}

// confirmStaging enforces the never-implicit staging gate: honored non-interactively
// via --yes or SDEV_CONFIRM=staging, else it prompts for the literal word "staging".
func confirmStaging(assumeYes bool) bool {
	if assumeYes || os.Getenv("SDEV_CONFIRM") == "staging" {
		logf("staging confirmed (non-interactive)")
		return true
	}
	fmt.Fprintln(os.Stderr, "⚠️  STAGING - this task targets your real staging environment.")
	fmt.Fprintln(os.Stderr, "    It can read/write REAL staging data.")
	fmt.Fprint(os.Stderr, "    Type 'staging' to continue: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line) == "staging"
}

// setAppEnv rewrites (or appends) the APP_ENV line in a task's .env.
func setAppEnv(dir, profile string) error {
	envPath := filepath.Join(dir, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "APP_ENV=") {
			lines[i] = "APP_ENV=" + profile
			found = true
			break
		}
	}
	out := strings.Join(lines, "\n")
	if !found {
		out += "APP_ENV=" + profile + "\n"
	}
	return os.WriteFile(envPath, []byte(out), 0o644)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

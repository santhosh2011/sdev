package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/paths"
)

// startResult is the --json success payload of `sdev start`.
type startResult struct {
	Task      string   `json:"task"`
	Project   string   `json:"project"`
	Env       string   `json:"env"`
	URL       string   `json:"url"`
	NginxPort int      `json:"nginx_port"`
	Created   bool     `json:"created"`
	Next      []string `json:"next"`
}

// startError is the --json payload when the stack fails to boot (exit 2).
type startError struct {
	Error   startErrorBody `json:"error"`
	Task    string         `json:"task"`
	Created bool           `json:"created"`
}

type startErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

// Start implements `sdev start <slug> [new flags] [--no-open] [--json]`: the
// front door that collapses create-or-resume, boot, and open into one command.
// It delegates task creation to `sdev new` and the docker boot to `sdev up`
// (reusing the staging guard + profile handling). Mirrors bin/start.
func Start(args []string) int {
	jsonOut, noOpen := false, false
	var passthru []string
	for _, a := range args {
		switch a {
		case "--json":
			jsonOut = true
		case "--no-open":
			noOpen = true
		default:
			passthru = append(passthru, a)
		}
	}

	slug := firstPositional(passthru)
	if slug == "" {
		return failMsg("slug required (usage: sdev start <slug> [new-task flags] [--no-open] [--json])")
	}

	home := paths.Home()
	project := config.ActiveProject()
	sdev := filepath.Join(paths.Install(), "bin", "sdev")

	taskDir, created := resolveOrCreate(home, project, slug, passthru, sdev)
	if created && taskDir == "" {
		return failMsg("create failed for '" + slug + "'")
	}

	if err := runUp(sdev, project, slug, passthru); err != nil {
		return startBootFailed(jsonOut, project, slug, created)
	}

	port := readEnvValue(taskDir, "NGINX_HOST_PORT")
	envProfile := readEnvValue(taskDir, "APP_ENV")
	url := ""
	if port != "" {
		url = "http://localhost:" + port + "/"
	}

	if jsonOut {
		return printStartJSON(project, slug, envProfile, url, port, created)
	}
	return printStartHuman(project, slug, url, created, noOpen)
}

// resolveOrCreate returns an existing task dir, or creates the task via `sdev new`
// (passing through every non-start flag). The bool reports whether it was
// created; on a creation failure it returns ("", true).
func resolveOrCreate(home, project, slug string, passthru []string, sdev string) (string, bool) {
	if ns := filepath.Join(home, "projects", project, slug); fsutil.IsDir(ns) {
		return ns, false
	}
	if flat := filepath.Join(home, "projects", slug); fsutil.IsDir(flat) {
		return flat, false
	}
	cmd := exec.Command(sdev, append([]string{"-p", project, "new"}, passthru...)...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr // new is chatty; keep stdout clean for --json
	if err := cmd.Run(); err != nil {
		return "", true
	}
	return filepath.Join(home, "projects", project, slug), true
}

// runUp boots the stack through `sdev up`, forwarding a --yes/-y confirmation.
func runUp(sdev, project, slug string, passthru []string) error {
	upArgs := []string{"-p", project, "up", slug}
	for _, a := range passthru {
		if a == "--yes" || a == "-y" {
			upArgs = append(upArgs, "--yes")
		}
	}
	cmd := exec.Command(sdev, upArgs...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

func startBootFailed(jsonOut bool, project, slug string, created bool) int {
	task := project + "/" + slug
	if jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(startError{
			Error:   startErrorBody{Code: "up_failed", Message: "stack failed to start", Hint: "sdev logs " + slug},
			Task:    task,
			Created: created,
		})
	} else {
		fmt.Fprintf(os.Stderr, "sdev start: stack failed to start for %s (task left in place - see: sdev logs %s)\n", task, slug)
	}
	return 2
}

func printStartJSON(project, slug, envProfile, url, port string, created bool) int {
	nginxPort := 0
	if port != "" {
		fmt.Sscanf(port, "%d", &nginxPort)
	}
	data, _ := json.MarshalIndent(startResult{
		Task:      project + "/" + slug,
		Project:   project,
		Env:       envProfile,
		URL:       url,
		NginxPort: nginxPort,
		Created:   created,
		Next:      []string{"sdev logs " + slug + "  # follow logs"},
	}, "", "  ")
	fmt.Println(string(data))
	return 0
}

func printStartHuman(project, slug, url string, created, noOpen bool) int {
	label := "resumed"
	if created {
		label = "created"
	}
	suffix := ""
	if url != "" {
		suffix = " -> " + url
	}
	fmt.Printf("sdev start: %s %s/%s%s\n", label, project, slug, suffix)
	if url != "" {
		if !noOpen && commandExists("open") {
			_ = exec.Command("open", url).Run()
		} else {
			fmt.Println(url)
		}
	}
	return 0
}

func firstPositional(args []string) string {
	for _, a := range args {
		if a != "" && !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

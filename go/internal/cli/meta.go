package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/session"
)

// Use implements `sdev use [<project>]`: with no arg it prints the active project;
// with one it pins that project for this terminal. Mirrors the use block in bin/sdev.
func Use(args []string) int {
	home := paths.Home()
	if len(args) == 0 || args[0] == "" {
		fmt.Printf("active project: %s (session: %s)\n", config.ResolveProject(home, ""), session.Key())
		return 0
	}
	target, err := config.RequireProject(home, args[0])
	if err != nil {
		return failErr(err)
	}
	if err := os.MkdirAll(session.Dir(), 0o755); err != nil {
		return failErr(err)
	}
	if err := os.WriteFile(session.Pointer(), []byte(target+"\n"), 0o644); err != nil {
		return failErr(err)
	}
	logf("pinned project '%s' for this terminal", target)
	return 0
}

// Projects implements `sdev projects`: list defined projects with their task counts.
func Projects(args []string) int {
	home := paths.Home()
	fmt.Printf("%-16s %-8s %-8s\n", "PROJECT", "TASKS", "RUNNING")
	names, _ := config.Projects(home)
	for _, p := range names {
		fmt.Printf("%-16s %-8s %-8s\n", p, strconv.Itoa(countTasks(home, p)), "-")
	}
	return 0
}

// countTasks counts a project's task dirs (those with a .env).
func countTasks(home, project string) int {
	entries, err := os.ReadDir(filepath.Join(home, "projects", project))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && fsutil.FileExists(filepath.Join(home, "projects", project, e.Name(), ".env")) {
			n++
		}
	}
	return n
}

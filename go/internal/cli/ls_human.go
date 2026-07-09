package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/santhosh2011/sdev/internal/dockerx"
	"github.com/santhosh2011/sdev/internal/envfile"
	"github.com/santhosh2011/sdev/internal/proc"
	"github.com/santhosh2011/sdev/internal/state"
)

// printLSHuman renders the human `sdev ls` listing: alive tasks, workspace-less
// leases, the warm pool, archived tasks, and orphan volumes. It reads the ledger
// for reservation annotations and pool/lease state.
func printLSHuman(home, scope string) {
	ledger, err := state.Load(state.FilePath(home))
	if err != nil {
		ledger = &state.Ledger{Tasks: map[string]state.Task{}}
	}

	fmt.Println("=== Alive tasks ===")
	if !printAliveHuman(home, scope, ledger) {
		fmt.Println("  (none)")
	}
	printLeasesHuman(home, scope, ledger)

	fmt.Println()
	fmt.Println("=== Warm pool ===")
	if !printPoolHuman(scope, ledger) {
		fmt.Println("  (none)")
	}

	fmt.Println()
	fmt.Println("=== Archived tasks ===")
	if !printArchivedHuman(home) {
		fmt.Println("  (none)")
	}

	fmt.Println()
	fmt.Println("=== Orphan docker volumes ===")
	if !printOrphansHuman(home) {
		fmt.Println("  (none)")
	}
}

func printAliveHuman(home, scope string, ledger *state.Ledger) bool {
	found := false
	projectsDir := filepath.Join(home, "projects")
	for _, name := range sortedDirNames(projectsDir) {
		if name == "_archive" {
			continue
		}
		flatEnv := filepath.Join(projectsDir, name, ".env")
		if fileExists(flatEnv) {
			if scope != "" && scope != legacyDefaultProject {
				continue
			}
			printTaskHuman(name+" (legacy)", flatEnv, name, ledger)
			found = true
			continue
		}
		if scope != "" && scope != name {
			continue
		}
		for _, slug := range sortedDirNames(filepath.Join(projectsDir, name)) {
			env := filepath.Join(projectsDir, name, slug, ".env")
			if fileExists(env) {
				key := name + "/" + slug
				printTaskHuman(key, env, key, ledger)
				found = true
			}
		}
	}
	return found
}

func printTaskHuman(label, envPath, key string, ledger *state.Ledger) {
	nginxPort := envfile.Value(envPath, "NGINX_HOST_PORT")
	if nginxPort == "" {
		nginxPort = "-"
	}
	offset := envfile.Value(envPath, "PORT_OFFSET")
	if offset == "" {
		offset = "-"
	}
	compose := envfile.Value(envPath, "COMPOSE_PROJECT_NAME")
	if compose == "" {
		compose = label
	}
	status := "stopped"
	if running := dockerx.RunningByComposeProject(compose); running > 0 {
		status = fmt.Sprintf("running (%d containers)", running)
	}
	suffix := ""
	if resv := taskLabel(ledger, key); resv != "" {
		suffix = " [" + resv + "]"
	}
	fmt.Printf("  %-28s offset=%-3s nginx=http://localhost:%-5s [%s]%s\n",
		label, offset, nginxPort, status, suffix)
}

func printLeasesHuman(home, scope string, ledger *state.Ledger) {
	shown := false
	for _, key := range ledger.LeasedKeys() {
		if isDirPath(filepath.Join(home, "projects", key)) {
			continue // already listed among alive tasks
		}
		if scope != "" && key != scope && !strings.HasPrefix(key, scope+"/") {
			continue
		}
		if !shown {
			fmt.Println()
			fmt.Println("=== Leases (no workspace) ===")
			shown = true
		}
		fmt.Printf("  %-28s [%s]\n", key, taskLabel(ledger, key))
	}
}

func printPoolHuman(scope string, ledger *state.Ledger) bool {
	found := false
	for _, p := range ledger.Pool {
		if p.Path == "" {
			continue
		}
		if scope != "" && p.Project != scope {
			continue
		}
		found = true
		status := "ok"
		if !isDirPath(p.Path) {
			status = "MISSING"
		}
		fmt.Printf("  %-28s %s [%s]\n", p.Project+"/"+p.Repo, p.Path, status)
	}
	return found
}

func printArchivedHuman(home string) bool {
	found := false
	archiveDir := filepath.Join(home, "projects", "_archive")
	for _, name := range sortedDirNames(archiveDir) {
		if info := filepath.Join(archiveDir, name, "ARCHIVE_INFO.md"); fileExists(info) {
			printArchivedLine(name+" (legacy)", info)
			found = true
			continue
		}
		for _, slug := range sortedDirNames(filepath.Join(archiveDir, name)) {
			info := filepath.Join(archiveDir, name, slug, "ARCHIVE_INFO.md")
			printArchivedLine(name+"/"+slug, info)
			found = true
		}
	}
	return found
}

func printArchivedLine(label, infoPath string) {
	date := archiveDate(infoPath)
	if date == "" {
		date = "-"
	}
	fmt.Printf("  %-28s archived %s\n", label, date)
}

func printOrphansHuman(home string) bool {
	known := knownProjectNames(home)
	found := false
	for _, vol := range dockerx.Volumes() {
		project := vol
		if i := strings.Index(vol, "_"); i >= 0 {
			project = vol[:i]
		}
		if !known[project] {
			fmt.Println("  " + vol)
			found = true
		}
	}
	return found
}

// taskLabel is the reservation annotation for a ledger key, or "" if the key is
// absent (mirrors task_status_label's has(key) guard).
func taskLabel(ledger *state.Ledger, key string) string {
	t, ok := ledger.Tasks[key]
	if !ok {
		return ""
	}
	return t.StatusLabel(proc.Alive)
}

package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh2011/sdev/internal/dockerx"
	"github.com/santhosh2011/sdev/internal/envfile"
	"github.com/santhosh2011/sdev/internal/jsonout"
	"github.com/santhosh2011/sdev/internal/paths"
)

// legacyDefaultProject is the implicit project a legacy flat task belongs to.
const legacyDefaultProject = "default"

// VolumeLister lists docker volume names; injected for hermetic tests.
type VolumeLister func() []string

// LS implements `sdev ls [--project <p>] [--json]`: a machine-readable report
// with --json, else the human listing.
func LS(args []string) int {
	scope := ""
	jsonOut := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if i+1 < len(args) {
				scope = args[i+1]
				i++
			}
		case "--json":
			jsonOut = true
		}
	}

	home := paths.Home()
	if !jsonOut {
		printLSHuman(home, scope)
		return 0
	}

	report := BuildLSReport(home, scope, dockerx.RunningByComposeProject, dockerx.Volumes)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "sdev ls: %v\n", err)
		return 1
	}
	return 0
}

// BuildLSReport lists alive + archived tasks and orphan volumes. An empty scope
// lists all projects (report.Project is null); a non-empty scope narrows the
// alive tasks to that project (archived tasks are never scoped).
func BuildLSReport(home, scope string, running RunningCounter, volumes VolumeLister) jsonout.LSReport {
	alive := walkAlive(home, scope, running)
	archived := walkArchived(home)
	orphans := findOrphans(home, volumes)

	runningTotal := 0
	for _, a := range alive {
		runningTotal += a.Running
	}

	var project *string
	if scope != "" {
		project = &scope
	}
	return jsonout.LSReport{
		Project:       project,
		Alive:         alive,
		Archived:      archived,
		OrphanVolumes: orphans,
		Totals: jsonout.LSTotals{
			Alive:    len(alive),
			Archived: len(archived),
			Orphans:  len(orphans),
			Running:  runningTotal,
		},
	}
}

func walkAlive(home, scope string, running RunningCounter) []jsonout.LSAlive {
	alive := []jsonout.LSAlive{}
	projectsDir := filepath.Join(home, "projects")
	for _, name := range sortedDirNames(projectsDir) {
		if name == "_archive" {
			continue
		}
		// Legacy flat task: projects/<name>/.env (implicit "default" project).
		flatEnv := filepath.Join(projectsDir, name, ".env")
		if fileExists(flatEnv) {
			if scope != "" && scope != legacyDefaultProject {
				continue
			}
			alive = append(alive, aliveEntry(name, flatEnv, running))
			continue
		}
		// Namespaced project dir.
		if scope != "" && scope != name {
			continue
		}
		projDir := filepath.Join(projectsDir, name)
		for _, slug := range sortedDirNames(projDir) {
			env := filepath.Join(projDir, slug, ".env")
			if fileExists(env) {
				alive = append(alive, aliveEntry(name+"/"+slug, env, running))
			}
		}
	}
	return alive
}

func aliveEntry(taskName, envPath string, running RunningCounter) jsonout.LSAlive {
	nginxPort := atoiOrZero(envfile.Value(envPath, "NGINX_HOST_PORT"))
	offset := atoiOrZero(envfile.Value(envPath, "PORT_OFFSET"))
	compose := envfile.Value(envPath, "COMPOSE_PROJECT_NAME")
	if compose == "" {
		compose = taskName
	}
	run := running(compose)
	status := "stopped"
	if run > 0 {
		status = "running"
	}
	url := ""
	if nginxPort > 0 {
		url = "http://localhost:" + strconv.Itoa(nginxPort) + "/"
	}
	return jsonout.LSAlive{
		Task:      taskName,
		Offset:    offset,
		NginxPort: nginxPort,
		URL:       url,
		Running:   run,
		Status:    status,
	}
}

func walkArchived(home string) []jsonout.LSArchived {
	archived := []jsonout.LSArchived{}
	archiveDir := filepath.Join(home, "projects", "_archive")
	for _, name := range sortedDirNames(archiveDir) {
		// Legacy flat archive: _archive/<name>/ARCHIVE_INFO.md.
		if info := filepath.Join(archiveDir, name, "ARCHIVE_INFO.md"); fileExists(info) {
			archived = append(archived, jsonout.LSArchived{Task: name, Archived: archiveDate(info)})
			continue
		}
		projDir := filepath.Join(archiveDir, name)
		for _, slug := range sortedDirNames(projDir) {
			info := filepath.Join(projDir, slug, "ARCHIVE_INFO.md")
			archived = append(archived, jsonout.LSArchived{Task: name + "/" + slug, Archived: archiveDate(info)})
		}
	}
	return archived
}

// sortedDirNames returns the immediate subdirectory names of dir in shell-glob
// order: each name is compared with a trailing "/" so it matches how bin/sdev's
// `*/` glob orders task dirs (e.g. "btp-http-registry" before "btp", because "-"
// sorts before "/"). This keeps the ported JSON array order identical to bash.
func sortedDirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		return names[i]+"/" < names[j]+"/"
	})
	return names
}

func findOrphans(home string, volumes VolumeLister) []string {
	orphans := []string{}
	known := knownProjectNames(home)
	for _, vol := range volumes() {
		project := vol
		if i := strings.Index(vol, "_"); i >= 0 {
			project = vol[:i]
		}
		if !known[project] {
			orphans = append(orphans, vol)
		}
	}
	return orphans
}

// knownProjectNames is the set of live + archived project dir names (minus the
// _archive container itself), used to tell orphan volumes from live ones.
func knownProjectNames(home string) map[string]bool {
	known := map[string]bool{}
	dirs := []string{
		filepath.Join(home, "projects"),
		filepath.Join(home, "projects", "_archive"),
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name() == "_archive" {
				continue
			}
			known[e.Name()] = true
		}
	}
	return known
}

// archiveDate reads the "- archive_date:" line from an ARCHIVE_INFO.md, or "".
func archiveDate(infoPath string) string {
	f, err := os.Open(infoPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "- archive_date:") {
			_, after, _ := strings.Cut(line, ":")
			return strings.TrimSpace(after)
		}
	}
	return ""
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

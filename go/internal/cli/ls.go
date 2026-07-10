package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh2011/sdev/internal/dockerx"
	"github.com/santhosh2011/sdev/internal/envfile"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/jsonout"
	"github.com/santhosh2011/sdev/internal/paths"
)

// legacyDefaultProject is the implicit project a legacy flat task belongs to.
const legacyDefaultProject = "default"

// On-disk layout segments used by the ls walkers.
const (
	projectsRoot    = "projects"
	archiveDirName  = "_archive"
	envFile         = ".env"
	archiveInfoFile = "ARCHIVE_INFO.md"
)

// VolumeLister lists docker volume names; injected for hermetic tests.
type VolumeLister func() []string

// aliveTask is a live task discovered on disk, before output formatting. The
// JSON and human `ls` paths both derive their rows from this, so the on-disk
// walk lives in one place.
type aliveTask struct {
	Key     string // ledger key: "<project>/<slug>" or a legacy "<name>"
	Label   string // display label: Key, or "<name> (legacy)" for a flat task
	Offset  int
	Nginx   int
	Running int
}

// archivedTask is an archived task discovered on disk.
type archivedTask struct {
	Key   string
	Label string
	Date  string
}

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
	return writeJSON("sdev ls", BuildLSReport(home, scope, dockerx.RunningByComposeProject, dockerx.Volumes))
}

// BuildLSReport lists alive + archived tasks and orphan volumes. An empty scope
// lists all projects (report.Project is null); a non-empty scope narrows the
// alive tasks to that project (archived tasks are never scoped).
func BuildLSReport(home, scope string, running RunningCounter, volumes VolumeLister) jsonout.LSReport {
	alive := make([]jsonout.LSAlive, 0)
	runningTotal := 0
	for _, a := range collectAlive(home, scope, running) {
		alive = append(alive, jsonout.LSAlive{
			Task:      a.Key,
			Offset:    a.Offset,
			NginxPort: a.Nginx,
			URL:       localhostURL(a.Nginx),
			Running:   a.Running,
			Status:    statusJSON(a.Running),
		})
		runningTotal += a.Running
	}

	archived := make([]jsonout.LSArchived, 0)
	for _, a := range collectArchived(home) {
		archived = append(archived, jsonout.LSArchived{Task: a.Key, Archived: a.Date})
	}

	orphans := findOrphans(home, volumes)

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

// collectAlive walks projects/ once (in shell-glob order) and returns the live
// tasks, applying the scope filter. Legacy flat tasks belong to "default".
func collectAlive(home, scope string, running RunningCounter) []aliveTask {
	tasks := []aliveTask{}
	projectsDir := filepath.Join(home, projectsRoot)
	for _, name := range sortedDirNames(projectsDir) {
		if name == archiveDirName {
			continue
		}
		if flatEnv := filepath.Join(projectsDir, name, envFile); fsutil.FileExists(flatEnv) {
			if scope != "" && scope != legacyDefaultProject {
				continue
			}
			tasks = append(tasks, newAliveTask(name, name+" (legacy)", flatEnv, running))
			continue
		}
		if scope != "" && scope != name {
			continue
		}
		projDir := filepath.Join(projectsDir, name)
		for _, slug := range sortedDirNames(projDir) {
			env := filepath.Join(projDir, slug, envFile)
			if fsutil.FileExists(env) {
				key := name + "/" + slug
				tasks = append(tasks, newAliveTask(key, key, env, running))
			}
		}
	}
	return tasks
}

func newAliveTask(key, label, envPath string, running RunningCounter) aliveTask {
	compose := envfile.Value(envPath, "COMPOSE_PROJECT_NAME")
	if compose == "" {
		compose = key
	}
	return aliveTask{
		Key:     key,
		Label:   label,
		Offset:  atoiOrZero(envfile.Value(envPath, "PORT_OFFSET")),
		Nginx:   atoiOrZero(envfile.Value(envPath, "NGINX_HOST_PORT")),
		Running: running(compose),
	}
}

// collectArchived walks projects/_archive/ once and returns the archived tasks.
func collectArchived(home string) []archivedTask {
	tasks := []archivedTask{}
	archiveDir := filepath.Join(home, projectsRoot, archiveDirName)
	for _, name := range sortedDirNames(archiveDir) {
		if info := filepath.Join(archiveDir, name, archiveInfoFile); fsutil.FileExists(info) {
			tasks = append(tasks, archivedTask{Key: name, Label: name + " (legacy)", Date: archiveDate(info)})
			continue
		}
		projDir := filepath.Join(archiveDir, name)
		for _, slug := range sortedDirNames(projDir) {
			key := name + "/" + slug
			info := filepath.Join(projDir, slug, archiveInfoFile)
			tasks = append(tasks, archivedTask{Key: key, Label: key, Date: archiveDate(info)})
		}
	}
	return tasks
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
	for _, dir := range []string{
		filepath.Join(home, projectsRoot),
		filepath.Join(home, projectsRoot, archiveDirName),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name() != archiveDirName {
				known[e.Name()] = true
			}
		}
	}
	return known
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

// archiveDate reads the "- archive_date:" line from an ARCHIVE_INFO.md, or "".
func archiveDate(infoPath string) string {
	f, err := os.Open(infoPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := scanner.Text(); strings.HasPrefix(line, "- archive_date:") {
			_, after, _ := strings.Cut(line, ":")
			return strings.TrimSpace(after)
		}
	}
	return ""
}

// localhostURL is the task URL for an nginx host port, or "" when unset.
func localhostURL(port int) string {
	if port <= 0 {
		return ""
	}
	return "http://localhost:" + strconv.Itoa(port) + "/"
}

func statusJSON(running int) string {
	if running > 0 {
		return "running"
	}
	return "stopped"
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

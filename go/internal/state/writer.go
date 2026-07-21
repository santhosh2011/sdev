package state

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/santhosh2011/sdev/internal/envfile"
	"github.com/santhosh2011/sdev/internal/fsutil"
)

// Init creates the ledger skeleton (and pool dir) if absent. Idempotent.
func Init(home string) error {
	if err := os.MkdirAll(PoolDir(home), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(FilePath(home)); err == nil {
		return nil
	}
	return Save(home, &Ledger{Version: 1, Tasks: map[string]Task{}, Pool: []PoolEntry{}, CoreStacks: map[string]CoreStack{}})
}

// Save atomically writes the ledger: marshal to a temp file in the state dir,
// then rename over the real file so a concurrent reader never sees a partial
// write. The caller holds the state lock.
func Save(home string, l *Ledger) error {
	if l.Tasks == nil {
		l.Tasks = map[string]Task{}
	}
	if l.Pool == nil {
		l.Pool = []PoolEntry{}
	}
	if l.CoreStacks == nil {
		l.CoreStacks = map[string]CoreStack{}
	}
	data, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	dir := Dir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state.*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, FilePath(home))
}

// Reservation is the intent recorded when allocating an offset: the task key
// plus its optional durable lease / ephemeral flags.
type Reservation struct {
	Key       string
	Lease     bool
	Holder    string
	Ephemeral bool
}

// AllocateOffset reserves the first free port offset (multiple of step) for the
// reservation and records it, after running the full seed + reconcile cycle. The
// caller MUST hold the state lock. Mirrors _allocate_offset_locked in bin/_lib.sh.
func AllocateOffset(home string, r Reservation, step int, alive ProcAlive) (int, error) {
	if err := Init(home); err != nil {
		return 0, err
	}
	l, err := Load(FilePath(home))
	if err != nil {
		return 0, err
	}
	seedFromEnv(home, l)
	reconcile(home, l, alive)

	used := usedOffsets(home, l)
	candidate := step
	for used[candidate] {
		candidate += step
	}
	l.Tasks[r.Key] = Task{
		Offset:      candidate,
		CreatedAt:   nowUTC(),
		Lease:       r.Lease,
		LeaseHolder: r.Holder,
		Ephemeral:   r.Ephemeral,
	}
	if err := Save(home, l); err != nil {
		return 0, err
	}
	return candidate, nil
}

// FreeTask drops a task's reservation (offset + lease + lock). Caller holds lock.
func FreeTask(home, key string) error {
	if err := Init(home); err != nil {
		return err
	}
	l, err := Load(FilePath(home))
	if err != nil {
		return err
	}
	delete(l.Tasks, key)
	return Save(home, l)
}

// seedFromEnv one-time-adopts existing .env PORT_OFFSETs so a fresh ledger never
// hands out an already-used offset. Idempotent via l.Seeded.
func seedFromEnv(home string, l *Ledger) {
	if l.Seeded {
		return
	}
	for _, env := range LiveEnvPaths(home) {
		off := envOffset(env)
		if off < 0 {
			continue
		}
		key := KeyFromEnv(home, env)
		if key == "" {
			continue
		}
		if _, ok := l.Tasks[key]; !ok {
			l.Tasks[key] = Task{Offset: off, CreatedAt: nowUTC()}
		}
	}
	l.Seeded = true
}

// reconcile drops tasks whose workspace is gone AND that are neither leased nor
// live-locked - freeing their offsets. This is the self-heal.
func reconcile(home string, l *Ledger, alive ProcAlive) {
	for key, t := range l.Tasks {
		if fsutil.IsDir(filepath.Join(home, projectsDir, key)) {
			continue // workspace present - keep
		}
		if t.Lease {
			continue // durable reservation - keep
		}
		if t.Pid != 0 && alive(t.Pid, t.ProcToken) {
			continue // live process-lock - keep
		}
		delete(l.Tasks, key)
	}
}

// usedOffsets is the set of reserved offsets: ledger task offsets, core-stack
// offsets (the reserved high band), and a fresh .env scan (belt-and-suspenders
// against an on-disk task missing from the ledger).
func usedOffsets(home string, l *Ledger) map[int]bool {
	used := map[int]bool{}
	for _, t := range l.Tasks {
		used[t.Offset] = true
	}
	for _, c := range l.CoreStacks {
		used[c.Offset] = true
	}
	for _, env := range LiveEnvPaths(home) {
		if off := envOffset(env); off >= 0 {
			used[off] = true
		}
	}
	return used
}

// LiveEnvPaths lists task .env files under projects/, excluding _archive.
func LiveEnvPaths(home string) []string {
	root := filepath.Join(home, projectsDir)
	var envs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == archiveName {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == envFileName {
			envs = append(envs, path)
		}
		return nil
	})
	sort.Strings(envs)
	return envs
}

// KeyFromEnv derives a ledger key ("<project>/<slug>" or a legacy "<slug>") from
// a task .env path, mirroring _state_key_from_env in bin/_lib.sh.
func KeyFromEnv(home, envPath string) string {
	rel := strings.TrimPrefix(envPath, filepath.Join(home, projectsDir)+string(filepath.Separator))
	return strings.TrimSuffix(rel, string(filepath.Separator)+envFileName)
}

func envOffset(envPath string) int {
	v := strings.TrimSpace(envfile.Value(envPath, "PORT_OFFSET"))
	if v == "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}

func nowUTC() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

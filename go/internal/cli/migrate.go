package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/state"
)

// Migrate implements `sdev migrate --from <old> [--force]`: copy an old in-repo
// sdev layout (project registries, confs, repo clones/symlinks, live workspaces)
// into $SDEV_HOME. Mirrors bin/migrate.
func Migrate(args []string) int {
	from, force := "", false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--from":
			if i+1 >= len(args) {
				return failMsg("--from requires a path argument")
			}
			from = args[i+1]
			i++
		case "--force":
			force = true
		case "-h", "--help":
			fmt.Println("Usage: sdev migrate --from <old-sdev-dir> [--force]")
			return 0
		default:
			return failMsg("unexpected arg: " + args[i])
		}
	}
	if from == "" {
		return failMsg("--from <old-sdev-dir> required")
	}
	if !dirExists(from) {
		return failMsg("--from dir not found: " + from)
	}
	from, _ = filepath.Abs(from)

	home := paths.Home()
	ensureHome(home)
	if from == home {
		return failMsg(fmt.Sprintf("--from is the same as SDEV_HOME (%s)", home))
	}

	projectsDir := filepath.Join(home, "core", "projects.d")
	if !force && populatedProjects(projectsDir) {
		return failMsg(home + " already has projects - pass --force to merge")
	}

	moved := 0
	_ = os.MkdirAll(projectsDir, 0o755)
	for _, f := range glob(filepath.Join(from, "core", "projects.d", "*.yml")) {
		if filepath.Base(f) == "example.yml" {
			continue
		}
		if err := copyFile(f, filepath.Join(projectsDir, filepath.Base(f)), 0o644); err == nil {
			logf("migrated core/projects.d/%s", filepath.Base(f))
			moved++
		}
	}

	moved += copyIn(from, home, "core/.task-config.yml")
	moved += copyIn(from, home, "core/.task-config.local.yml")
	for _, d := range subdirs(filepath.Join(from, "core")) {
		if d == "projects.d" {
			continue
		}
		moved += copyIn(from, home, "core/"+d)
	}
	for _, d := range subdirs(filepath.Join(from, "confs")) {
		moved += copyIn(from, home, "confs/"+d)
	}
	projectsMigrated := 0
	for _, d := range subdirs(filepath.Join(from, "projects")) {
		if d == "_archive" {
			continue
		}
		moved += copyIn(from, home, "projects/"+d)
		projectsMigrated++
	}
	if projectsMigrated > 0 {
		logf("note: migrated live task worktrees reference the OLD repo and may be broken - recreate tasks with 'sdev new' after verifying")
	}
	logf("migration complete - %d item(s) into %s", moved, home)
	return 0
}

// ensureHome creates the SDEV_HOME skeleton and seeds the global config from the
// bundled default. Mirrors ensure_home in bin/_lib.sh.
func ensureHome(home string) {
	for _, d := range []string{
		filepath.Join(home, "core", "projects.d"),
		filepath.Join(home, "confs"),
		filepath.Join(home, "projects", "_archive"),
		state.Dir(home),
		state.PoolDir(home),
	} {
		_ = os.MkdirAll(d, 0o755)
	}
	global := filepath.Join(home, "core", ".task-config.yml")
	seed := filepath.Join(paths.Install(), "core", ".task-config.yml")
	if !fileExists(global) && fileExists(seed) {
		_ = copyFile(seed, global, 0o644)
	}
}

// copyIn copies a path relative to `from` into `home`, merging directory contents
// (no nesting) and preserving inner symlinks. Returns 1 if anything was copied.
func copyIn(from, home, rel string) int {
	src := filepath.Join(from, rel)
	dst := filepath.Join(home, rel)
	info, err := os.Lstat(src)
	if err != nil {
		return 0
	}
	if info.IsDir() {
		if err := copyTree(src, dst); err != nil {
			return 0
		}
	} else {
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if err := copyFile(src, dst, info.Mode().Perm()); err != nil {
			return 0
		}
	}
	logf("migrated %s", rel)
	return 1
}

// copyTree copies the contents of src into dst, creating dst, preserving symlinks
// as symlinks (matching `cp -R src/. dst/`).
func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		info, err := os.Lstat(s)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(s)
			if err != nil {
				return err
			}
			_ = os.Remove(d)
			if err := os.Symlink(target, d); err != nil {
				return err
			}
		case info.IsDir():
			if err := copyTree(s, d); err != nil {
				return err
			}
		default:
			if err := copyFile(s, d, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

func populatedProjects(projectsDir string) bool {
	for _, f := range glob(filepath.Join(projectsDir, "*.yml")) {
		if filepath.Base(f) != "example.yml" {
			return true
		}
	}
	return false
}

func subdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		// Follow symlinks-to-dirs too (repo sources may be symlinked).
		if info, err := os.Stat(filepath.Join(dir, e.Name())); err == nil && info.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func glob(pattern string) []string {
	m, _ := filepath.Glob(pattern)
	return m
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

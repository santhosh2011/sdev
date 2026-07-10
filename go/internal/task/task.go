// Package task resolves a task slug to its workspace directory, mirroring
// require_task_dir in bin/sdev.
package task

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/santhosh2011/sdev/internal/fsutil"
)

// Resolve returns the workspace dir for slug in project: the namespaced
// projects/<project>/<slug>, else the legacy flat projects/<slug>.
func Resolve(home, project, slug string) (string, error) {
	if slug == "" {
		return "", errors.New("slug required")
	}
	namespaced := filepath.Join(home, "projects", project, slug)
	if fsutil.IsDir(namespaced) {
		return namespaced, nil
	}
	flat := filepath.Join(home, "projects", slug)
	if fsutil.IsDir(flat) {
		return flat, nil
	}
	return "", fmt.Errorf("task %q not found in project %q (or legacy projects/%s)", slug, project, slug)
}

// Key returns the ledger key for a resolved task dir: the path relative to
// projects/ ("<project>/<slug>" or a legacy "<slug>").
func Key(home, dir string) string {
	return strings.TrimPrefix(dir, filepath.Join(home, "projects")+string(filepath.Separator))
}

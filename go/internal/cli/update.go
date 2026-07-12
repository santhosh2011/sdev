package cli

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/santhosh2011/sdev/internal/paths"
)

// Update implements `sdev update`: hand off to the bundled self-update engine,
// which ships at the install-dir root (next to VERSION, alongside bin/). Mirrors
// the update block in bin/sdev.
func Update(args []string) int {
	updater := filepath.Join(paths.Install(), "self-update")
	if !isExecutable(updater) {
		return failMsg("self-update not found at " + updater + " (reinstall sdev)")
	}
	if err := syscall.Exec(updater, append([]string{updater}, args...), os.Environ()); err != nil {
		return failErr(err)
	}
	return 0 // unreachable: Exec replaces the process on success
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

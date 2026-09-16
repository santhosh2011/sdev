// Package infra bridges opted-in workspace lifecycle to the bash implementation.
// It carries no duplicate database-name or SQL logic. Old snapshots bypass it.
package infra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/envfile"
	"github.com/santhosh2011/sdev/internal/paths"
)

func Shared(dir string) bool {
	return envfile.Value(filepath.Join(dir, ".env"), "SDEV_POSTGRES_FLAVOR") != ""
}

func command(home, action, key string) *exec.Cmd {
	cmd := exec.Command(filepath.Join(paths.Install(), "bin", "infra-task"), action, key)
	cmd.Env = append(os.Environ(), "SDEV_HOME="+home)
	cmd.Stderr = os.Stderr
	return cmd
}

// WriteEnv snapshots opt-in at creation time; it never upgrades existing tasks.
func WriteEnv(home, project, key, dir string) error {
	if config.PostgresFlavor(home, project) == "" {
		return nil
	}
	data, err := command(home, "env", key).Output()
	if err != nil {
		return fmt.Errorf("shared Postgres config: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, ".env"), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func Ensure(home, key, dir string) error {
	if !Shared(dir) {
		return nil
	}
	cmd := command(home, "ensure", key)
	cmd.Stdout = os.Stderr
	return cmd.Run()
}

// Drop returns errors before callers remove the snapshot, worktrees or ledger.
func Drop(home, key string) error {
	if !Shared(filepath.Join(home, "projects", key)) {
		return nil
	}
	cmd := command(home, "drop", key)
	cmd.Stdout = os.Stderr
	return cmd.Run()
}

// Package paths resolves sdev's data root, mirroring bin/_lib.sh precedence.
package paths

import (
	"os"
	"path/filepath"
)

// Home returns SDEV_HOME using the same precedence as bin/_lib.sh: an explicit
// $SDEV_HOME, then $WORKSPACE_ROOT (the legacy/test alias for a combined root),
// then ~/.sdev.
func Home() string {
	if h := os.Getenv("SDEV_HOME"); h != "" {
		return h
	}
	if w := os.Getenv("WORKSPACE_ROOT"); w != "" {
		return w
	}
	return filepath.Join(os.Getenv("HOME"), ".sdev")
}

// Install returns SDEV_INSTALL, the tool code root (parent of bin/) that holds
// the bundled templates and Claude hooks. It mirrors bin/_lib.sh, which derives
// it as the parent of the script dir; here that is the parent of the directory
// holding the running sdev-go binary. An explicit $SDEV_INSTALL wins (tests).
func Install() string {
	if i := os.Getenv("SDEV_INSTALL"); i != "" {
		return i
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(filepath.Dir(exe))
	}
	return ""
}

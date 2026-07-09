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

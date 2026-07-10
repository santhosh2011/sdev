// Package fsutil holds the small filesystem predicates shared across sdev's
// packages.
package fsutil

import "os"

// IsDir reports whether p exists and is a directory.
func IsDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// FileExists reports whether p exists and is a regular (non-directory) file.
func FileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

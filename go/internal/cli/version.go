package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh2011/sdev/internal/paths"
)

// version is a build-time fallback (settable via -ldflags "-X ...cli.version=<v>");
// the shipped VERSION file takes precedence at runtime.
var version = "dev"

// Version implements `sdev version`: print the shipped VERSION (with any
// release-please annotation stripped), falling back to the build-time version.
func Version(_ []string) int {
	fmt.Println(resolveVersion())
	return 0
}

func resolveVersion() string {
	if v := readVersionFile(filepath.Join(paths.Install(), "VERSION")); v != "" {
		return v
	}
	return version
}

// readVersionFile returns the first line of a VERSION file with any release-please
// annotation comment and surrounding whitespace stripped, or "" if unreadable.
func readVersionFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line := string(data)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if i := strings.IndexByte(line, '#'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

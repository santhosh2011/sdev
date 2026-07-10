// Package envfile reads KEY=value pairs from a task .env file.
package envfile

import (
	"bufio"
	"os"
	"strings"
)

// Value returns the value of key in the .env file at path, or "" if the file is
// unreadable or the key is absent. First match wins, mirroring `grep ... | head`.
func Value(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	prefix := key + "="
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	return ""
}

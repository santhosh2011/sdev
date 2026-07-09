// Package dockerx counts running compose containers. It degrades to 0 when
// docker is absent or errors, mirroring the bash `docker ps ... || true`.
package dockerx

import (
	"os/exec"
	"strings"
)

// RunningByComposeProject returns the number of running containers labelled with
// the given compose project name, or 0 on any error (docker missing, daemon
// down, no match).
func RunningByComposeProject(composeProject string) int {
	if composeProject == "" {
		return 0
	}
	out, err := exec.Command("docker", "ps",
		"--filter", "label=com.docker.compose.project="+composeProject, "-q").Output()
	if err != nil {
		return 0
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

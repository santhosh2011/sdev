package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/santhosh2011/sdev/internal/compose"
	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/envfile"
	"github.com/santhosh2011/sdev/internal/jsonout"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/task"
)

// PS implements `sdev ps [--json] <slug> [extra compose args]`.
func PS(args []string) int {
	jsonOut := false
	slug := ""
	var extra []string
	for _, a := range args {
		switch {
		case a == "--json":
			jsonOut = true
		case slug == "":
			slug = a
		default:
			extra = append(extra, a)
		}
	}

	dir, err := task.Resolve(paths.Home(), config.ActiveProject(), slug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sdev ps: %v\n", err)
		return 1
	}

	if jsonOut {
		return psJSON(paths.Home(), dir)
	}
	return psHuman(dir, extra)
}

func psJSON(home, dir string) int {
	url := localhostURL(atoiOrZero(envfile.Value(filepath.Join(dir, ".env"), "NGINX_HOST_PORT")))

	cmd := exec.Command("./compose", "ps", "--format", "json")
	cmd.Dir = dir
	out, _ := cmd.Output() // degrade: any error -> empty output -> empty services

	report := jsonout.PSReport{
		Task:     task.Key(home, dir),
		URL:      url,
		Services: compose.ParsePS(out),
	}
	return writeJSON("sdev ps", report)
}

func psHuman(dir string, extra []string) int {
	cmd := exec.Command("./compose", append([]string{"ps"}, extra...)...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "sdev ps: %v\n", err)
		return 1
	}
	return 0
}

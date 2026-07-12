// Package cli implements sdev's command dispatch. It is both the router for the
// standalone Go entrypoint (parsing -p, resolving the active project, and running
// the implicit `sdev <slug>` shorthand) and the target the bash router bin/sdev
// execs per subcommand during the strangler migration.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/paths"
)

// Run parses the project flag, resolves and exports the active project, then
// dispatches the subcommand. A missing subcommand prints usage; a non-command
// first argument is the implicit `new <slug>` shorthand. Returns the exit code.
func Run(args []string) int {
	flag, rest := extractProjectFlag(args)
	project := config.ResolveProject(paths.Home(), flag)
	// Export so every downstream config.ActiveProject() (and any bash fallback we
	// might exec, e.g. `sdev up` from start) sees the resolved project.
	_ = os.Setenv("SDEV_PROJECT", project)

	if len(rest) == 0 {
		return Dashboard(nil) // no-arg axi surface; `sdev help` prints full usage
	}
	cmd, sub := rest[0], rest[1:]

	if code, ok := dispatch(cmd, sub); ok {
		return code
	}

	// Not a known command: a flag-like token is an error; anything else is the
	// implicit `new <slug>` shorthand.
	if strings.HasPrefix(cmd, "-") {
		usage()
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		return 1
	}
	return Newtask(append([]string{cmd}, sub...))
}

// dispatch runs a known subcommand, returning its exit code and true; it returns
// (0, false) when cmd is not a recognized command.
func dispatch(cmd string, sub []string) (int, bool) {
	switch cmd {
	case "help", "-h", "--help":
		usage()
		return 0, true
	case "version":
		return Version(sub), true
	case "use":
		return Use(sub), true
	case "projects":
		return Projects(sub), true
	case "migrate":
		return Migrate(sub), true
	case "review":
		return Review(sub), true
	case "ship":
		return Ship(sub), true
	case "init":
		return Init(sub), true
	case "edit":
		return Edit(sub), true
	case "update":
		return Update(sub), true
	case "setup":
		return Setup(sub), true
	case "status":
		return Status(sub), true
	case "ps":
		return PS(sub), true
	case "ls", "list":
		return LS(sub), true
	case "doctor":
		return Doctor(sub), true
	case "alloc":
		return Alloc(sub), true
	case "lease":
		return Lease(sub), true
	case "release":
		return Release(sub), true
	case "hold":
		return Hold(sub), true
	case "destroy":
		return Destroy(sub), true
	case "prune":
		return Prune(sub), true
	case "new":
		return Newtask(sub), true
	case "end":
		return End(sub), true
	case "start":
		return Start(sub), true
	case "up":
		return Up(sub), true
	case "down":
		return Down(sub), true
	case "nuke":
		return Nuke(sub), true
	case "logs":
		return Logs(sub), true
	case "shell":
		return Shell(sub), true
	case "open":
		return Open(sub), true
	case "code":
		return Code(sub), true
	case "cd":
		return Cd(sub), true
	}
	return 0, false
}

// extractProjectFlag pulls a leading -p/--project (and its value) off the front of
// args, returning the project (or "") and the remaining args from the subcommand
// on. Only the leading flag is consumed: a subcommand's own --project (e.g. the
// scope forwarded to `ls`) is left in place for the subcommand to parse.
func extractProjectFlag(args []string) (string, []string) {
	project := ""
	i := 0
	for i < len(args) {
		if args[i] != "-p" && args[i] != "--project" {
			break // first non-project token is the subcommand
		}
		if i+1 < len(args) {
			project = args[i+1]
			i += 2
		} else {
			i++
		}
	}
	return project, args[i:]
}

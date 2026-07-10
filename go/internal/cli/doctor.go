package cli

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/santhosh2011/sdev/internal/envfile"
	"github.com/santhosh2011/sdev/internal/fsutil"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/proc"
	"github.com/santhosh2011/sdev/internal/state"
)

// wOK is the write-permission mode for syscall.Access.
const wOK = 0x02

// Doctor implements `sdev doctor`: read-only diagnostics for sdev's environment
// and central ledger. Exits non-zero if any FAIL is reported (WARNs are fine).
func Doctor(_ []string) int {
	home := paths.Home()
	d := &doctorRun{}

	fmt.Println("=== dependencies ===")
	d.checkBash()
	d.checkYq()
	d.checkJq()

	fmt.Println()
	fmt.Println("=== paths ===")
	d.checkPaths(home)

	fmt.Println()
	fmt.Println("=== state ledger ===")
	d.checkLedger(home)

	fmt.Println()
	if d.failed {
		fmt.Println("doctor: problems found (see FAIL lines above)")
		return 1
	}
	fmt.Println("doctor: OK")
	return 0
}

// ANSI colors for the doctor status prefixes.
const (
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorReset  = "\033[0m"
)

type doctorRun struct{ failed bool }

func (d *doctorRun) pass(msg string) { fmt.Printf("  %sok%s   %s\n", colorGreen, colorReset, msg) }
func (d *doctorRun) warn(msg string) { fmt.Printf("  %swarn%s %s\n", colorYellow, colorReset, msg) }
func (d *doctorRun) fail(msg string) {
	fmt.Printf("  %sFAIL%s %s\n", colorRed, colorReset, msg)
	d.failed = true
}

func (d *doctorRun) checkBash() {
	out, err := exec.Command("bash", "-c", `echo "$BASH_VERSION"`).Output()
	if err != nil {
		d.fail("bash not found on PATH - brew install bash")
		return
	}
	ver := strings.TrimSpace(string(out))
	display := ver
	if i := strings.IndexByte(ver, '('); i >= 0 {
		display = ver[:i]
	}
	major := 0
	if i := strings.IndexByte(ver, '.'); i >= 0 {
		major, _ = strconv.Atoi(ver[:i])
	}
	if major >= 4 {
		d.pass(fmt.Sprintf("bash %s (>= 4)", display))
	} else {
		d.fail(fmt.Sprintf("bash %s is < 4 - brew install bash", display))
	}
}

var yqV4 = regexp.MustCompile(`v?4\.`)

func (d *doctorRun) checkYq() {
	out, err := exec.Command("yq", "--version").Output()
	if err != nil {
		d.fail("yq not found on PATH - brew install yq")
		return
	}
	ver := strings.TrimSpace(string(out))
	if strings.Contains(ver, "mikefarah") {
		if yqV4.MatchString(ver) {
			d.pass("yq " + ver)
		} else {
			d.fail("yq is not v4 (" + ver + ") - sdev needs mikefarah yq v4")
		}
		return
	}
	// Older mikefarah builds omit the URL; accept a v4.x version string.
	if regexp.MustCompile(`version v?4\.`).MatchString(ver) {
		d.pass("yq " + ver)
	} else {
		d.fail("yq present but not recognizable as mikefarah v4 (" + ver + ")")
	}
}

func (d *doctorRun) checkJq() {
	out, err := exec.Command("jq", "--version").Output()
	if err != nil {
		d.warn("jq not found - needed for --json output (sdev status/ls/ps --json)")
		return
	}
	d.pass("jq " + strings.TrimSpace(string(out)))
}

func (d *doctorRun) checkPaths(home string) {
	if fsutil.IsDir(home) {
		d.pass("SDEV_HOME=" + home)
	} else {
		d.warn("SDEV_HOME=" + home + " does not exist yet (run 'sdev init')")
	}
	if writable(home) {
		d.pass("SDEV_HOME writable")
	} else {
		d.fail("SDEV_HOME not writable: " + home)
	}
}

func (d *doctorRun) checkLedger(home string) {
	stateFile := state.FilePath(home)
	if !fsutil.FileExists(stateFile) {
		d.warn("no ledger yet at " + stateFile + " (created on first 'sdev new')")
		return
	}
	ledger, err := state.Load(stateFile)
	if err != nil {
		d.fail("ledger is not valid YAML: " + stateFile)
		return
	}
	d.pass("ledger parses: " + stateFile)

	d.checkLock(home)
	d.checkOffsetDrift(home, ledger)
	d.checkDuplicateOffsets(ledger)
	d.checkPool(ledger)
}

func (d *doctorRun) checkLock(home string) {
	lock := state.LockPath(home)
	if fi, err := os.Lstat(lock); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(lock)
		pid := target
		if i := strings.IndexByte(target, ':'); i >= 0 {
			pid = target[:i]
		}
		n, _ := strconv.Atoi(pid)
		if pid != "" && proc.Alive(n, "") {
			d.warn(fmt.Sprintf("state lock currently held by live pid %s", pid))
		} else {
			shown := pid
			if shown == "" {
				shown = "?"
			}
			d.warn(fmt.Sprintf("stale state lock present (holder pid '%s' dead) - self-heals on next use, or: rm -f '%s'", shown, lock))
		}
		return
	}
	if _, err := os.Lstat(lock); err == nil {
		d.warn("legacy state lock present - self-heals on next use, or: rm -rf '" + lock + "'")
		return
	}
	d.pass("state lock free")
}

func (d *doctorRun) checkOffsetDrift(home string, ledger *state.Ledger) {
	drift := false
	for _, e := range state.LiveEnvPaths(home) {
		off := envfile.Value(e, "PORT_OFFSET")
		if off == "" {
			continue
		}
		key := state.KeyFromEnv(home, e)
		t, ok := ledger.Tasks[key]
		if !ok {
			d.warn(fmt.Sprintf("task '%s' (offset %s on disk) is missing from the ledger - will be adopted on next allocation", key, off))
			drift = true
			continue
		}
		if strconv.Itoa(t.Offset) != strings.TrimSpace(off) {
			d.fail(fmt.Sprintf("offset drift for '%s': disk=%s ledger=%d", key, off, t.Offset))
			drift = true
		}
	}
	if !drift {
		d.pass("ledger offsets match on-disk tasks")
	}
}

func (d *doctorRun) checkDuplicateOffsets(ledger *state.Ledger) {
	dupes := duplicateOffsets(ledger)
	if len(dupes) > 0 {
		parts := make([]string, len(dupes))
		for i, o := range dupes {
			parts[i] = strconv.Itoa(o)
		}
		d.fail("duplicate offsets in ledger: " + strings.Join(parts, " "))
		return
	}
	d.pass("no duplicate offsets in ledger")
}

func (d *doctorRun) checkPool(ledger *state.Ledger) {
	bad := false
	for _, p := range ledger.Pool {
		if p.Path != "" && !fsutil.IsDir(p.Path) {
			d.warn("pool entry missing on disk: " + p.Path + " (prune with 'sdev end' bookkeeping or edit the ledger)")
			bad = true
		}
	}
	if !bad {
		d.pass("warm-pool entries present on disk")
	}
}

// duplicateOffsets returns the offsets assigned to more than one task, sorted.
func duplicateOffsets(l *state.Ledger) []int {
	counts := map[int]int{}
	for _, t := range l.Tasks {
		counts[t.Offset]++
	}
	dupes := []int{}
	for off, n := range counts {
		if n > 1 {
			dupes = append(dupes, off)
		}
	}
	sort.Ints(dupes)
	return dupes
}

func writable(p string) bool {
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return true
	}
	return syscall.Access(p, wOK) == nil
}

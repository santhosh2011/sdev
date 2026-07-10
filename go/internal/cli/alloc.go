package cli

import (
	"fmt"
	"os"

	"github.com/santhosh2011/sdev/internal/config"
	"github.com/santhosh2011/sdev/internal/lock"
	"github.com/santhosh2011/sdev/internal/paths"
	"github.com/santhosh2011/sdev/internal/proc"
	"github.com/santhosh2011/sdev/internal/state"
)

// Alloc implements the internal `sdev-go alloc <key> [--lease] [--holder <h>]
// [--ephemeral]`: reserve the first free port offset for key under the state
// lock and print it. It is not routed from bin/sdev - it is the Go allocation
// primitive that slice-3 mutators (and the lock-interop test) drive, and it
// contends on the same lock + ledger as the bash allocate_offset.
func Alloc(args []string) int {
	key, holder := "", ""
	lease, ephemeral := false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--lease":
			lease = true
		case "--ephemeral":
			ephemeral = true
		case "--holder":
			if i+1 < len(args) {
				holder = args[i+1]
				i++
			}
		default:
			if key == "" {
				key = args[i]
			}
		}
	}
	if key == "" {
		fmt.Fprintln(os.Stderr, "sdev-go alloc: key required")
		return 2
	}

	home := paths.Home()
	step := config.PortStep(home)
	offset := 0
	reservation := state.Reservation{Key: key, Lease: lease, Holder: holder, Ephemeral: ephemeral}
	err := lock.With(state.Dir(home), func() error {
		o, e := state.AllocateOffset(home, reservation, step, proc.Alive)
		offset = o
		return e
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "sdev-go alloc: %v\n", err)
		return 1
	}
	fmt.Println(offset)
	return 0
}

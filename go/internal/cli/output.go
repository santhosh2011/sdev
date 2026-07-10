package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

// writeJSON encodes v as indented JSON to stdout. On error it reports it under
// cmd and returns exit code 1; otherwise 0.
func writeJSON(cmd string, v any) int {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cmd, err)
		return 1
	}
	return 0
}

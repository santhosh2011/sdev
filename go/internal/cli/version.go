package cli

import "fmt"

// version is overridable at build time via -ldflags "-X ...cli.version=<v>";
// local builds report "dev".
var version = "dev"

// Version implements `sdev-go version`.
func Version(_ []string) int {
	fmt.Println(version)
	return 0
}

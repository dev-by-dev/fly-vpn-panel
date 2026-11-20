// flypanel is the VPN control panel binary. Full serve wiring lands in a
// later task; only version reporting and a serve stub exist for now.
package main

import (
	"fmt"
	"os"
)

const version = "v0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("flypanel " + version)
	case "serve":
		fmt.Fprintln(os.Stderr, "serve: not implemented yet")
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: flypanel <version|serve>")
}

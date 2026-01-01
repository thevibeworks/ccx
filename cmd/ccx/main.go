package main

import (
	"fmt"
	"os"

	"github.com/thevibeworks/ccx/internal/cmd"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, buildTime)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "ccx: %v\n", err)
		os.Exit(1)
	}
}

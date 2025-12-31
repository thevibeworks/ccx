package main

import (
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
		os.Exit(1)
	}
}

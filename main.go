package main

import (
	"os"

	"github.com/artisan-build/hitch/internal/cmd"
	"github.com/artisan-build/hitch/internal/harness"
)

func main() {
	os.Exit(cmd.Main(os.Args[1:], os.Stdout, os.Stderr, harness.CurrentEnv))
}

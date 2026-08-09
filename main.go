package main

import (
	"os"

	"github.com/artisan-build/hitch/internal/cmd"
)

func main() {
	if err := cmd.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

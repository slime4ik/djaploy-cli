package main

import (
	"os"

	"github.com/slime4ik/djaploy-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}

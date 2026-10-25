package main

import (
	"os"

	"github.com/m-mattia-m/mail-stats/internal/cli"
)

// version is set at build time via -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	os.Exit(cli.Execute(version))
}

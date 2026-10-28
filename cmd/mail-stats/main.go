package main

import (
	"os"
	// Embeds the timezone database, so TZ (which decides the calendar day of
	// the busiest day) also works in the container image, which has none.
	_ "time/tzdata"

	"github.com/m-mattia-m/mail-stats/internal/cli"
)

// version is set at build time via -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	os.Exit(cli.Execute(version))
}

package main

import (
	"fmt"
	"log/slog"
	"os"

	cli "github.com/jawher/mow.cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	app := cli.App("dehub", "A lightweight and secure debugging lib for remote process.")

	app.Version("v version", fmt.Sprintf("v%s, commit %s, built at %s", version, commit, date))

	setupHubCLI(app)
	setupServantCLI(app)
	setupMasterCLI(app)

	err := app.Run(os.Args)
	if err != nil {
		slog.Error(err.Error())
		cli.Exit(1)
	}
}

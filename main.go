package main

import (
	"fmt"
	"log/slog"
	"os"

	cli "github.com/jawher/mow.cli"
	"github.com/lmittmann/tint"
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

func output(jsonOutput bool) *slog.Logger {
	if jsonOutput {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(tint.NewHandler(os.Stdout, nil))
}

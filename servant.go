package main

import (
	cli "github.com/jawher/mow.cli"
	dehub "github.com/ysmood/dehub/lib"
)

type servantConf struct {
	id         string
	hubAddr    string
	prvKey     string
	pubKeys    []string
	jsonOutput bool
}

func setupServantCLI(app *cli.Cli) {
	app.Command("s servant",
		"A client that connects to the hub server to follow the master client.",
		func(c *cli.Cmd) {
			var conf servantConf

			c.Spec = "-p -k... [OPTIONS] ID"

			c.StringOptPtr(&conf.hubAddr, "a addr", ":8813", "The address of the hub server.")
			c.StringArgPtr(&conf.id, "ID", "", "The id of the servant. It should be unique.")

			c.StringOptPtr(&conf.prvKey, "p private-key", "", "The private key file path.")
			c.StringsOptPtr(&conf.pubKeys, "k public-keys", nil, "The public key file paths.")

			c.BoolOptPtr(&conf.jsonOutput, "j json", true, "json output to stdout")

			c.Action = func() { runServant(conf) }
		})
}

func runServant(conf servantConf) {
	servant := dehub.NewServant(dehub.ServantID(conf.id), privateKey(conf.prvKey), publicKeys(conf.pubKeys)...)
	servant.Logger = output(conf.jsonOutput)

	servant.Handle(dial(conf.hubAddr))()
}

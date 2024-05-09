package main

import (
	cli "github.com/jawher/mow.cli"
	dehub "github.com/ysmood/dehub/lib"
)

type servantConf struct {
	id        string
	hubAddr   string
	websocket bool

	prvKey  string
	pubKeys []string

	jsonOutput bool
}

func setupServantCLI(app *cli.Cli) {
	app.Command("s servant",
		"A client that connects to the hub server to follow the master client.",
		func(c *cli.Cmd) {
			var conf servantConf

			c.Spec = "-p [OPTIONS] PUBLIC_KEYS..."

			c.StringOptPtr(&conf.hubAddr, "a addr", ":8813", "The address of the hub server.")
			c.StringOptPtr(&conf.id, "i id", id(), "The id of the servant. It should be unique.")
			c.BoolOptPtr(&conf.websocket, "w ws", false,
				"Use websocket to connect to hub. If set, the addr should be a websocket address.")

			c.StringOptPtr(&conf.prvKey, "p private-key", "", "The private key file path.")
			c.StringsArgPtr(&conf.pubKeys, "PUBLIC_KEYS", nil, "The list of public key content or path.")

			c.BoolOptPtr(&conf.jsonOutput, "j json", true, "json output to stdout")

			c.Action = func() { runServant(conf) }
		})
}

func runServant(conf servantConf) {
	servant := dehub.NewServant(dehub.ServantID(conf.id), privateKey(conf.prvKey), publicKeys(conf.pubKeys))
	servant.Logger = output(conf.jsonOutput)

	servant.Serve(dial(conf.websocket, conf.hubAddr))()
}

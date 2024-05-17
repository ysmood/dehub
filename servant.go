package main

import (
	"time"

	cli "github.com/jawher/mow.cli"
	dehub "github.com/ysmood/dehub/lib"
)

type servantConf struct {
	id            string
	hubAddr       string
	websocket     bool
	retryInterval RetryInterval

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

			c.StringOptPtr(&conf.hubAddr, "a addr", "dehub.ysmood.org:8813", "The address of the hub server.")
			c.StringOptPtr(&conf.id, "i id", id(), "The id of the servant. It should be unique.")
			c.BoolOptPtr(&conf.websocket, "w ws", false,
				"Use websocket to connect to hub. If set, the addr should be a websocket address.")
			c.VarOpt("r retry-interval", &conf.retryInterval, "The retry interval in seconds.")

			c.StringOptPtr(&conf.prvKey, "p private-key", "", "The private key file path.")
			c.StringsArgPtr(&conf.pubKeys, "PUBLIC_KEYS", nil,
				"The list of github user id, public key content, or path that are allowed to connect to the servant.")

			c.BoolOptPtr(&conf.jsonOutput, "j json", true, "json output to stdout")

			c.Action = func() { runServant(conf) }
		})
}

func runServant(conf servantConf) {
	logger := output(conf.jsonOutput)
	servant := dehub.NewServant(dehub.ServantID(conf.id), privateKey(conf.prvKey), publicKeys(logger, conf.pubKeys))
	servant.Logger = logger

	for {
		conn, err := dial(conf.websocket, conf.hubAddr)
		if err != nil {
			logger.Error("failed to connect to the hub", "err", err)
		} else {
			servant.Serve(conn)()
		}

		logger.Info("servant retries to connect to the hub", "wait", conf.retryInterval.String())

		time.Sleep(conf.retryInterval.Get())
	}
}

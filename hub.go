package main

import (
	"net"

	cli "github.com/jawher/mow.cli"
	dehub "github.com/ysmood/dehub/lib"
	"github.com/ysmood/myip"
)

type hubConf struct {
	jsonOutput  bool
	localhostIP bool
	addr        string
	websocket   bool
}

func setupHubCLI(app *cli.Cli) {
	app.Command("h hub", "A hub server for connecting the master and servant client.", func(c *cli.Cmd) {
		var conf hubConf

		c.StringOptPtr(&conf.addr, "a addr", ":8813", "The address the hub server listens to.")
		c.BoolOptPtr(&conf.localhostIP, "local-ip", false,
			"Use 127.0.0.1 as the ip address for the hub server. If false it will use the interface IP.")
		c.BoolOptPtr(&conf.jsonOutput, "j json", true, "json output to stdout")
		c.BoolOptPtr(&conf.websocket, "w ws", false, "Handle each tcp connection as websocket.")

		c.Action = func() { runHub(conf) }
	})
}

func runHub(conf hubConf) {
	hub := dehub.NewHub()
	hub.Logger = output(conf.jsonOutput)
	hub.GetIP = func() (string, error) {
		if conf.localhostIP {
			return "127.0.0.1", nil
		}

		return myip.New().GetInterfaceIP()
	}

	hubSrv, err := net.Listen("tcp", conf.addr)
	e(err)

	hub.Logger.Info("hub server started", "addr", conf.addr)

	for {
		conn, err := hubSrv.Accept()
		if err != nil {
			return
		}

		if conf.websocket {
			err := dehub.WebsocketUpgrade(conn)
			if err != nil {
				hub.Logger.Error("failed to upgrade to websocket", "err", err)
				_ = conn.Close()

				continue
			}
		}

		go hub.Handle(conn)
	}
}

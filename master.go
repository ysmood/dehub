package main

import (
	"net"
	"os"
	"os/signal"

	cli "github.com/jawher/mow.cli"
	dehub "github.com/ysmood/dehub/lib"
	"github.com/ysmood/dehub/lib/utils"
)

type masterConf struct {
	id         string
	hubAddr    string
	prvKey     string
	pubKeys    []string
	jsonOutput bool

	socks5 string

	nfsAddr   string
	remoteDir string
	localDir  string

	cmdName string
	cmdArgs []string
}

func setupMasterCLI(app *cli.Cli) {
	app.Command("m master",
		"A client that connects to the hub server to command the servant client.",
		func(c *cli.Cmd) {
			var conf masterConf

			c.Spec = "-p -k... [OPTIONS] ID"

			c.StringArgPtr(&conf.id, "ID", "", "The id of the servant to command.")
			c.StringOptPtr(&conf.hubAddr, "a addr", ":8813", "The address of the hub server.")

			c.StringOptPtr(&conf.prvKey, "p private-key", "", "The private key file path.")
			c.StringsOptPtr(&conf.pubKeys, "k public-keys", nil, "The public key file paths.")

			c.BoolOptPtr(&conf.jsonOutput, "j json", false, "json output to stdout")

			c.StringOptPtr(&conf.socks5, "s socks5", "", "The address of the socks5 server.")

			c.StringOptPtr(&conf.nfsAddr, "n nfs-addr", "", "The address of the nfs server.")
			c.StringOptPtr(&conf.remoteDir, "r remote-dir", ".", "The remote directory to serve.")
			c.StringOptPtr(&conf.localDir, "l local-dir", "", "The local directory to sync.")

			c.StringOptPtr(&conf.cmdName, "c cmd", "", "The command to run.")
			c.StringsOptPtr(&conf.cmdArgs, "g cmd-args", nil, "The arguments of the command.")

			c.Action = func() { runMaster(conf) }
		})
}

func runMaster(conf masterConf) {
	master := dehub.NewMaster(dehub.ServantID(conf.id), privateKey(conf.prvKey), publicKeys(conf.pubKeys)...)
	master.Logger = output(conf.jsonOutput)

	e(master.Connect(dial(conf.hubAddr)))

	// Forward socks5
	if conf.socks5 != "" {
		l, err := net.Listen("tcp", conf.socks5)
		e(err)

		master.Logger.Info("socks5 server on", "addr", l.Addr().String())

		go func() { e(master.ForwardSocks5(l)) }()
	}

	// Forward dir
	if conf.nfsAddr != "" {
		fsSrv, err := net.Listen("tcp", conf.nfsAddr)
		e(err)

		go func() { e(master.ServeNFS(conf.remoteDir, fsSrv, 0)) }()

		master.Logger.Info("nfs server on", "addr", fsSrv.Addr().String())

		localDir := conf.localDir
		if localDir == "" {
			localDir, err = os.MkdirTemp("", "dehub-nfs")
			e(err)
		}

		e(utils.MountNFS(fsSrv.Addr().(*net.TCPAddr), localDir))
		defer func() {
			e(utils.UnmountNFS(localDir))
			master.Logger.Info("nfs unmounted", "dir", localDir)
		}()
		master.Logger.Info("nfs mounted", "dir", localDir)
	}

	// Run remote shell command
	if conf.cmdName != "" {
		master.Logger.Info("run command", "cmd", conf.cmdName, "args", conf.cmdArgs)

		e(master.Exec(os.Stdin, os.Stdout, conf.cmdName, conf.cmdArgs...))
	} else {
		// Capture CTRL+C
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
	}
}

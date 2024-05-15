package main

import (
	"net"
	"os"
	"os/signal"

	cli "github.com/jawher/mow.cli"
	dehub "github.com/ysmood/dehub/lib"
	"github.com/ysmood/dehub/lib/utils"
	"golang.org/x/crypto/ssh"
)

type masterConf struct {
	id        string
	hubAddr   string
	websocket bool

	prvKey  string
	pubKeys []string

	outputFile string

	socks5    string
	httpProxy string

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

			c.Spec = "[OPTIONS] ID_PREFIX [-- CMD [CMD_ARGS...]]"

			c.StringArgPtr(&conf.id, "ID_PREFIX", "", "The id prefix of the servant to command, "+
				"it will connect to the first servant id that match the id prefix.")
			c.StringOptPtr(&conf.hubAddr, "a addr", "dehub.ysmood.org:8813", "The address of the hub server.")
			c.BoolOptPtr(&conf.websocket, "w ws", false,
				"Use websocket to connect to hub. If set, the addr should be a websocket address.")

			c.StringOptPtr(&conf.prvKey, "p private-key", "", "The private key file path.")
			c.StringsOptPtr(&conf.pubKeys, "k public-keys", nil, "The list of public key content or path.")

			c.StringOptPtr(&conf.outputFile, "o output", "tmp/dehub-master.log", "The file path to append the output.")

			c.StringOptPtr(&conf.socks5, "s socks5", "", "The address of the socks5 server.")
			c.StringOptPtr(&conf.httpProxy, "x http-proxy", "", "The address of the http proxy server.")

			c.StringOptPtr(&conf.nfsAddr, "n nfs-addr", "", "The address of the nfs server.")
			c.StringOptPtr(&conf.remoteDir, "r remote-dir", ".", "The remote directory to serve.")
			c.StringOptPtr(&conf.localDir, "l local-dir", "", "The local directory to sync.")

			c.StringOptPtr(&conf.cmdName, "c cmd", "", "The command to run.")
			c.StringArgPtr(&conf.cmdName, "CMD", "", "The command to run.")
			c.StringsArgPtr(&conf.cmdArgs, "CMD_ARGS", nil, "The arguments of the command.")

			c.Action = func() { runMaster(conf) }
		})
}

func runMaster(conf masterConf) { //nolint: funlen
	checkKey := publicKeys(conf.pubKeys)

	master := dehub.NewMaster(dehub.ServantID(conf.id), privateKey(conf.prvKey), func(key ssh.PublicKey) bool {
		if len(conf.pubKeys) == 0 {
			return readLine("Do you trust the servant public key:\n"+ssh.FingerprintSHA256(key)+"\n"+
				`Input ENTER to trust, input any other to abort: `) == ""
		}

		return checkKey(key)
	})
	master.Logger = output(false)

	e(master.Connect(mustDial(conf.websocket, conf.hubAddr)))

	wait := false

	// Forward socks5
	if conf.socks5 != "" {
		l, err := net.Listen("tcp", conf.socks5)
		e(err)

		master.Logger.Info("socks5 server on", "addr", l.Addr().String())

		go func() { e(master.ForwardSocks5(l)) }()

		wait = true
	}

	// Forward http
	if conf.httpProxy != "" {
		l, err := net.Listen("tcp", conf.httpProxy)
		e(err)

		master.Logger.Info("http proxy server on", "addr", l.Addr().String())

		go func() { e(master.ForwardHTTP(l)) }()

		wait = true
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

		wait = true
	}

	// Run remote shell command
	if conf.cmdName != "" {
		master.Logger.Info("run command", "cmd", conf.cmdName, "args", conf.cmdArgs)
		master.Logger.Info("output log to", "file", conf.outputFile)

		master.Logger = outputToFile(conf.outputFile)

		e(master.Exec(os.Stdin, os.Stdout, conf.cmdName, conf.cmdArgs...))
	} else if wait {
		// Capture CTRL+C
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
	}
}

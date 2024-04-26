package main

import (
	"log/slog"
	"net"
	"os"

	"github.com/lmittmann/tint"
	"github.com/ysmood/dehub"
	"github.com/ysmood/dehub/lib/utils"
)

func main() {
	hubAddr := "127.0.0.1:8813"

	master := dehub.NewMaster("test", readFile("lib/fixtures/id_ed25519"))
	master.Logger = slog.New(tint.NewHandler(os.Stdout, nil))

	// Forward socks5
	go func() {
		l, err := net.Listen("tcp", ":7777")
		E(err)

		E(master.ForwardSocks5(dial(hubAddr), l))
	}()

	// Forward dir
	{
		fsSrv, err := net.Listen("tcp", "127.0.0.1:0")
		E(err)

		go func() { E(master.ServeNFS(dial(hubAddr), "lib/fixtures", fsSrv, 0)) }()
		master.Logger.Info("nfs server on", "addr", fsSrv.Addr().String())

		localDir, err := os.MkdirTemp("", "dehub-nfs")
		E(err)

		E(utils.MountNFS(fsSrv.Addr().(*net.TCPAddr), localDir))
		defer func() {
			E(utils.UnmountNFS(localDir))
			master.Logger.Info("nfs unmounted", "dir", localDir)
		}()
		master.Logger.Info("nfs mounted", "dir", localDir)
	}

	// Forward shell
	E(master.Exec(dial(hubAddr), os.Stdin, os.Stdout, "sh"))
}

func dial(addr string) net.Conn {
	conn, err := net.Dial("tcp", addr)
	E(err)
	return conn
}

func E(err error) {
	if err != nil {
		panic(err)
	}
}

func readFile(path string) []byte {
	b, err := os.ReadFile(path)
	E(err)
	return b
}

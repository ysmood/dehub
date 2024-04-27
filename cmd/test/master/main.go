package main

import (
	"log/slog"
	"net"
	"os"

	"github.com/lmittmann/tint"
	"github.com/ysmood/dehub"
	shared "github.com/ysmood/dehub/cmd/test"
	"github.com/ysmood/dehub/lib/utils"
)

func main() {
	hubAddr := "127.0.0.1:8813"

	master := dehub.NewMaster("test", shared.PrivateKey(), shared.PublicKey())
	master.Logger = slog.New(tint.NewHandler(os.Stdout, nil))

	shared.E(master.Connect(shared.Dial(hubAddr)))

	// Forward socks5
	go func() {
		l, err := net.Listen("tcp", ":7777")
		shared.E(err)

		shared.E(master.ForwardSocks5(l))
	}()

	// Forward dir
	{
		fsSrv, err := net.Listen("tcp", "127.0.0.1:0")
		shared.E(err)

		go func() { shared.E(master.ServeNFS("lib/fixtures", fsSrv, 0)) }()
		master.Logger.Info("nfs server on", "addr", fsSrv.Addr().String())

		localDir, err := os.MkdirTemp("", "dehub-nfs")
		shared.E(err)

		shared.E(utils.MountNFS(fsSrv.Addr().(*net.TCPAddr), localDir))
		defer func() {
			shared.E(utils.UnmountNFS(localDir))
			master.Logger.Info("nfs unmounted", "dir", localDir)
		}()
		master.Logger.Info("nfs mounted", "dir", localDir)
	}

	// Forward shell
	shared.E(master.Exec(os.Stdin, os.Stdout, "sh"))
}

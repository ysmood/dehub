package main

import (
	"log/slog"
	"net"
	"os"

	"github.com/lmittmann/tint"
	"github.com/ysmood/dehub"
	shared "github.com/ysmood/dehub/cmd/test"
)

func main() {
	hub := dehub.NewHub()
	hub.Logger = slog.New(tint.NewHandler(os.Stdout, nil))
	hub.GetIP = func() (string, error) {
		return "127.0.0.1", nil
	}

	servant := dehub.NewServant("test", shared.PrivateKey(), shared.PublicKey())
	servant.Logger = slog.New(tint.NewHandler(os.Stdout, nil))

	hubSrv, err := net.Listen("tcp", ":8813")
	shared.E(err)

	hubAddr := hubSrv.Addr().String()

	go func() {
		for {
			conn, err := hubSrv.Accept()
			if err != nil {
				return
			}

			go hub.Handle(conn)
		}
	}()

	servant.Handle(shared.Dial(hubAddr))()
}

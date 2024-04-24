package main

import (
	"log/slog"
	"net"
	"os"

	"github.com/ysmood/dehub"
)

func main() {
	log := slog.NewJSONHandler(os.Stdout, nil)

	hub := dehub.NewHub(log)
	hub.GetIP = func() (string, error) {
		return "127.0.0.1", nil
	}

	servant := dehub.NewServant(log, "test", []string{string(readFile("lib/fixtures/id_ed25519.pub"))})

	hubSrv, err := net.Listen("tcp", ":8813")
	E(err)

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

	servant.Handle(dial(hubAddr))()
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

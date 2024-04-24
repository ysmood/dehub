package main

import (
	"io"
	"log/slog"
	"net"
	"os"

	"github.com/ysmood/dehub"
)

func main() {
	log := slog.NewJSONHandler(io.Discard, nil)

	hubAddr := "127.0.0.1:8813"

	master := dehub.NewMaster(log, "test", readFile("lib/fixtures/id_ed25519"))

	// Forward socks5
	go func() {
		l, err := net.Listen("tcp", ":7777")
		E(err)

		E(master.ForwardSocks5(dial(hubAddr), l))
	}()

	// Forward dir
	go func() {
		dir, err := os.MkdirTemp("", "dehub-nfs")
		E(err)

		E(master.ForwardDir(dial(hubAddr), "lib/fixtures", dir, 0))
	}()

	// Forward shell
	E(master.Exec(dial(hubAddr), os.Stdin, os.Stdout, "sh"))

	for _, conn := range connList {
		_ = conn.Close()
	}
}

var connList []net.Conn

func dial(addr string) net.Conn {
	conn, err := net.Dial("tcp", addr)
	E(err)

	connList = append(connList, conn)

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

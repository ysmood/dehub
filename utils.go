package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	cli "github.com/jawher/mow.cli"
	dehub "github.com/ysmood/dehub/lib"
	"golang.org/x/crypto/ssh"
)

const dialTimeout = time.Second * 10

func privateKey(path string) ssh.Signer {
	b := readFile(path)
	s, err := ssh.ParsePrivateKey(b)
	e(err)
	return s
}

func publicKeys(paths []string) [][]byte {
	list := [][]byte{}

	for _, path := range paths {
		list = append(list, readFile(path))
	}

	return list
}

func dial(websocket bool, addr string) net.Conn {
	if websocket {
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()

		if !strings.HasPrefix(addr, "ws") {
			e(fmt.Errorf("The '--addr' cli option should be a websocket url when '-w' is set: %s", addr))
		}

		conn, err := dehub.WebsocketDial(ctx, addr)
		e(err)
		return conn
	} else {
		conn, err := net.DialTimeout("tcp", addr, dialTimeout)
		e(err)
		return conn
	}
}

func e(err error) {
	if err != nil {
		fmt.Println(err.Error()) //nolint: forbidigo

		cli.Exit(2) //nolint: gomnd
	}
}

func readFile(path string) []byte {
	b, err := os.ReadFile(path)
	e(err)
	return b
}

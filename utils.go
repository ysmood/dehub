package main

import (
	"fmt"
	"net"
	"os"

	cli "github.com/jawher/mow.cli"
	"golang.org/x/crypto/ssh"
)

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

func dial(addr string) net.Conn {
	conn, err := net.Dial("tcp", addr)
	e(err)
	return conn
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

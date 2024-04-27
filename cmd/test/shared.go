package shared

import (
	"net"
	"os"

	"golang.org/x/crypto/ssh"
)

func PrivateKey() ssh.Signer {
	b := ReadFile("lib/fixtures/id_ed25519")
	s, err := ssh.ParsePrivateKey(b)
	E(err)
	return s
}

func PublicKey() []byte {
	b := ReadFile("lib/fixtures/id_ed25519.pub")
	return b
}

func Dial(addr string) net.Conn {
	conn, err := net.Dial("tcp", addr)
	E(err)
	return conn
}

func E(err error) {
	if err != nil {
		panic(err)
	}
}

func ReadFile(path string) []byte {
	b, err := os.ReadFile(path)
	E(err)
	return b
}

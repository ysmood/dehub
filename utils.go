package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	cli "github.com/jawher/mow.cli"
	"github.com/lmittmann/tint"
	dehub "github.com/ysmood/dehub/lib"
	"github.com/ysmood/whisper/lib/secure"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func output(jsonOutput bool) *slog.Logger {
	if jsonOutput {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(tint.NewHandler(os.Stdout, nil))
}

func outputToFile(path string) *slog.Logger {
	_ = os.MkdirAll(filepath.Dir(path), 0o755) //nolint: mnd,gomnd

	f, _ := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) //nolint: mnd,gomnd

	return slog.New(slog.NewTextHandler(f, nil))
}

const dialTimeout = time.Second * 10

func privateKey(path string) ssh.Signer {
	b := readFile(path)

	_, err := secure.SSHPrvKey(b, "")
	if err != nil {
		if secure.IsAuthErr(err) {
			p := getPassphrase(path)

			s, err := ssh.ParsePrivateKeyWithPassphrase(b, []byte(p))
			e(err)

			return s
		}

		e(err)
	}

	s, err := ssh.ParsePrivateKey(b)
	e(err)
	return s
}

func getPassphrase(location string) string {
	return readPassphrase(fmt.Sprintf("Enter passphrase for private key %s: ", location))
}

func readPassphrase(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)

	fd := int(os.Stdin.Fd())

	if !term.IsTerminal(fd) {
		e(errors.New("stdin is not a terminal"))
	}

	inputPass, err := term.ReadPassword(fd)
	if err != nil {
		e(err)
	}

	fmt.Fprintln(os.Stderr)

	return string(inputPass)
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

		cli.Exit(2) //nolint: mnd,gomnd
	}
}

func readFile(path string) []byte {
	b, err := os.ReadFile(path)
	e(err)
	return b
}

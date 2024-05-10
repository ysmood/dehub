package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
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
	_ = os.MkdirAll(filepath.Dir(path), 0o755) //nolint: mnd

	f, _ := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) //nolint: mnd

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

func readLine(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)

	reader := bufio.NewReader(os.Stdin)

	line, err := reader.ReadString('\n')
	if err != nil {
		e(err)
	}

	return strings.TrimSpace(line)
}

func publicKeys(keys []string) func(ssh.PublicKey) bool {
	list := [][]byte{}

	for _, key := range keys {
		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err == nil {
			list = append(list, []byte(key))
			continue
		}

		parseErr := err

		b := readFile(key)
		_, _, _, _, err = ssh.ParseAuthorizedKey(b)
		if err != nil {
			e(fmt.Errorf("failed get public key from '%s': %w, %w", key, parseErr, err))
		}

		list = append(list, b)
	}

	fn, err := dehub.CheckPublicKeys(list...)
	if err != nil {
		e(err)
	}

	return fn
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

		cli.Exit(2) //nolint: mnd
	}
}

func readFile(path string) []byte {
	b, err := os.ReadFile(path)
	e(err)
	return b
}

func id() string {
	b := make([]byte, 16) //nolint: mnd

	_, err := rand.Read(b)
	e(err)

	return hex.EncodeToString(b)
}

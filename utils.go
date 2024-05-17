package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	cli "github.com/jawher/mow.cli"
	"github.com/lmittmann/tint"
	dehub "github.com/ysmood/dehub/lib"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func output(jsonOutput bool) *slog.Logger {
	if jsonOutput {
		return slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	return slog.New(tint.NewHandler(os.Stderr, nil))
}

func outputToFile(path string) *slog.Logger {
	_ = os.MkdirAll(filepath.Dir(path), 0o755) //nolint: mnd

	f, _ := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) //nolint: mnd

	return slog.New(slog.NewTextHandler(f, nil))
}

const dialTimeout = time.Second * 10

func privateKey(path string) ssh.Signer {
	if path == "" {
		return nil
	}

	b := readFile(path)

	_, err := ssh.ParseRawPrivateKey(b)
	if err != nil {
		if isAuthErr(err) {
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

func isAuthErr(err error) bool {
	missingErr := &ssh.PassphraseMissingError{}
	return errors.Is(err, x509.IncorrectPasswordError) || errors.As(err, &missingErr)
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

func publicKeys(l *slog.Logger, keys []string) func(ssh.PublicKey) bool {
	list := [][]byte{}

	for _, key := range keys {
		if strings.HasPrefix(key, "@") {
			ks, err := getGithubPubkey(key[1:])
			if err != nil {
				l.Error("failed to get public key from github", "err", err)
				continue
			}

			list = append(list, ks...)
			continue
		}

		_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err == nil {
			list = append(list, []byte(key))
			continue
		}

		l.Info("key is not public key format, try treat it as file path", "key", key)

		b := readFile(key)
		_, _, _, _, err = ssh.ParseAuthorizedKey(b)
		if err != nil {
			l.Error("failed to parse public key", "key", key, "err", err)
			continue
		}

		list = append(list, b)
	}

	fn, err := dehub.CheckPublicKeys(list...)
	if err != nil {
		e(err)
	}

	return fn
}

func getGithubPubkey(userID string) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "", "https://api.github.com/users/"+userID+"/keys", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to get public keys: %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response of pubic key request: %w", err)
	}

	defer func() { _ = res.Body.Close() }()

	type key struct {
		Key string `json:"key"`
	}

	var keys []key
	err = json.NewDecoder(res.Body).Decode(&keys)
	if err != nil {
		return nil, fmt.Errorf("failed to decode public keys for user: %w", err)
	}

	list := [][]byte{}

	for _, k := range keys {
		list = append(list, []byte(k.Key))
	}

	return list, nil
}

func mustDial(websocket bool, addr string) net.Conn {
	conn, err := dial(websocket, addr)
	e(err)

	return conn
}

func dial(websocket bool, addr string) (net.Conn, error) {
	if websocket {
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()

		if !strings.HasPrefix(addr, "ws") {
			return nil, fmt.Errorf("The '--addr' cli option should be a websocket url when '-w' is set: %s", addr)
		}

		conn, err := dehub.WebsocketDial(ctx, addr)
		if err != nil {
			return nil, err
		}

		return conn, nil
	} else {
		conn, err := net.DialTimeout("tcp", addr, dialTimeout)
		if err != nil {
			return nil, err
		}
		return conn, nil
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

type RetryInterval time.Duration

func (d *RetryInterval) Set(v string) error {
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return err
	}
	*d = RetryInterval(parsed)
	return nil
}

func (d *RetryInterval) String() string {
	if *d == 0 {
		return "5s"
	}
	return time.Duration(*d).String()
}

func (d *RetryInterval) Get() time.Duration {
	if *d == 0 {
		_ = d.Set(d.String())
	}

	return time.Duration(*d)
}

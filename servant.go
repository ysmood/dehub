package dehub

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/hashicorp/yamux"
	"github.com/things-go/go-socks5"
	"github.com/willscott/go-nfs"
	nfshelper "github.com/willscott/go-nfs/helpers"
	osfsx "github.com/ysmood/dehub/lib/osfs"
	"golang.org/x/crypto/ssh"
)

const DefaultSignTimeout = 10 * time.Second

func (n ServantID) String() string {
	return string(n)
}

func NewServant(id ServantID, pubKeys []string) *Servant {
	keys := PubKeys{}
	for _, raw := range pubKeys {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
		if err != nil {
			panic(err)
		}

		hash, err := publicKeyHash(key)
		if err != nil {
			panic(err)
		}

		keys[hash] = PubKey{raw: strings.TrimSpace(raw), sshPubKey: key}
	}

	return &Servant{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		SignTimeout: DefaultSignTimeout,
		id:          id,
		pubKeys:     keys,
	}
}

func (s *Servant) Handle(conn net.Conn) func() {
	err := connectHub(conn, ClientTypeServant, s.id)
	if err != nil {
		s.Logger.Error("Failed to connect to hub", slog.Any("err", err))
		return func() {}
	}

	server, err := yamux.Server(conn, nil)
	if err != nil {
		s.Logger.Error("Failed to create yamux server", slog.Any("err", err))
		return func() {}
	}

	s.Logger.Info("servant connected to hub", slog.String("servant", s.id.String()))

	return func() {
		for {
			conn, err := server.Accept()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}

				s.Logger.Error("Failed to accept connection", slog.Any("err", err))
				return
			}

			go func() {
				defer func() { _ = conn.Close() }()

				err := s.startTunnel(conn)
				if err != nil {
					writeMsg(conn, err.Error())
				}
			}()
		}
	}
}

func (s *Servant) startTunnel(conn net.Conn) error {
	header, err := readMsg[TunnelHeader](conn)
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	if err := s.verifySign(header); err != nil {
		return fmt.Errorf("failed to verify sign: %w", err)
	}

	return s.serve(header, conn)
}

func (s *Servant) verifySign(header *TunnelHeader) error {
	var t time.Time

	err := json.Unmarshal(header.Timestamp, &t)
	if err != nil {
		return fmt.Errorf("invalid timestamp in header: %w", err)
	}

	if time.Since(t) > s.SignTimeout {
		return errors.New("timestamp expired")
	}

	key, ok := s.pubKeys[header.PubKeyHash]
	if !ok {
		s.Logger.Error("public key not found", slog.Any("hash", header.PubKeyHash))
		return errors.New("public key not found")
	}

	err = key.sshPubKey.Verify(header.Timestamp, header.Sign)
	if err != nil {
		return fmt.Errorf("failed to verify signature: %w", err)
	}

	s.Logger.Info("authorized",
		slog.String("key", key.raw),
		slog.String("hash", hex.EncodeToString(header.PubKeyHash[:])))

	return nil
}

func (s *Servant) serve(h *TunnelHeader, conn net.Conn) error {
	s.Logger.Info("master connected", slog.Any("command", h.Command))

	switch h.Command {
	case CommandExec:
		return s.exec(conn, h.ExecMeta)
	case CommandForwardSocks5:
		return s.forwardSocks5(conn)
	case CommandShareDir:
		return s.shareDir(conn, h.ShareDirMeta)
	}

	return fmt.Errorf("unknown command: %d", h.Command)
}

func (s *Servant) exec(conn net.Conn, meta *ExecMeta) error {
	c := exec.Command(meta.Cmd, meta.Args...)
	defer func() { _ = c.Process.Kill() }()

	p, err := pty.StartWithSize(c, meta.Size)
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}

	defer func() { _ = p.Close() }()

	startTunnel(conn)

	go func() { _, _ = io.Copy(p, conn) }()

	_, _ = io.Copy(conn, p)

	return nil
}

func (s *Servant) forwardSocks5(conn net.Conn) error {
	startTunnel(conn)

	tunnel, err := yamux.Server(conn, nil)
	if err != nil {
		s.Logger.Error("Failed to create yamux session", slog.Any("err", err))
		return nil
	}

	proxy := socks5.NewServer()

	for {
		stream, err := tunnel.AcceptStream()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			s.Logger.Error("Failed to accept stream", slog.Any("err", err))
			return nil
		}

		go func() {
			_ = proxy.ServeConn(stream)
		}()
	}
}

func (s *Servant) shareDir(conn net.Conn, meta *MountDirMeta) error {
	tunnel, err := yamux.Server(conn, nil)
	if err != nil {
		return fmt.Errorf("failed to create yamux tunnel: %w", err)
	}

	if _, err := os.Stat(meta.Path); os.IsNotExist(err) {
		return fmt.Errorf("remote directory does not exist: %w", err)
	}

	startTunnel(conn)

	bfs := osfs.New(meta.Path)
	bfsPlusChange := osfsx.New(bfs)

	handler := nfshelper.NewNullAuthHandler(bfsPlusChange)
	cacheHelper := nfshelper.NewCachingHandler(handler, meta.CacheLimit)

	nfs.Log.SetLevel(-1) // disable log
	err = nfs.Serve(tunnel, cacheHelper)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}

		s.Logger.Error("failed to serve nfs", "err", err)
	}

	return nil
}

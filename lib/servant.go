package dehub

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
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

func NewServant(id ServantID, prvKey ssh.Signer, check func(ssh.PublicKey) bool) *Servant {
	s := &Servant{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		id:     id,
	}

	s.sshConf = &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if check(key) {
				s.Logger.Info("authorized master public key",
					slog.String("session-id", hex.EncodeToString(conn.SessionID())),
					slog.String("master-pubkey", FormatPubKey(key)))

				return nil, nil //nolint: nilnil
			}

			return nil, errors.New("not a authorized master public key")
		},
	}

	s.sshConf.AddHostKey(prvKey)

	return s
}

func (s *Servant) Serve(conn io.ReadWriteCloser) func() {
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

	s.Logger.Info("servant connected to hub", slog.String("servantId", s.id.String()))

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

			go s.serve(conn)
		}
	}
}

func (s *Servant) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	session, err := yamux.Server(conn, nil)
	if err != nil {
		s.Logger.Error("Failed to create yamux session", slog.Any("err", err))
		return
	}

	tunnel, err := session.Accept()
	if err != nil {
		s.Logger.Error("Failed to accept tunnel", slog.Any("err", err))
		return
	}

	sshConn, channels, _, err := ssh.NewServerConn(tunnel, s.sshConf)
	if err != nil {
		s.Logger.Error("Failed to handshake", "err", err)
		return
	}

	go func() {
		<-session.CloseChan()
		s.Logger.Info("master disconnected", slog.String("session-id", hex.EncodeToString(sshConn.SessionID())))
	}()

	for newChan := range channels {
		switch Command(newChan.ChannelType()) {
		case CommandExec:
			go s.exec(newChan)
		case CommandForwardSocks5:
			go s.forwardSocks5(newChan)
		case CommandShareDir:
			go s.shareDir(newChan)
		}
	}
}

func (s *Servant) exec(newChan ssh.NewChannel) {
	var meta ExecMeta
	err := json.Unmarshal(newChan.ExtraData(), &meta)
	if err != nil {
		_ = newChan.Reject(UnmarshalMetaFailed, err.Error())
		return
	}

	c := exec.Command(meta.Cmd, meta.Args...)
	defer func() { _ = c.Process.Kill() }()

	p, err := pty.StartWithSize(c, meta.Size)
	if err != nil {
		_ = newChan.Reject(FailedStartPTY, err.Error())
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		s.Logger.Error("Failed to accept exec channel", "err", err)
		return
	}

	defer func() { _ = p.Close() }()

	go func() {
		for req := range reqs {
			if req.Type == ExecResizeRequest {
				var size pty.Winsize
				err := json.Unmarshal(req.Payload, &size)
				if err != nil {
					s.Logger.Error("Failed to unmarshal terminal size", "err", err)
					return
				}

				err = pty.Setsize(p, &size)
				if err != nil {
					s.Logger.Error("Failed to set terminal size", "err", err)
					return
				}
			} else {
				s.Logger.Error("Unknown exec request type", "req", req.Type)
			}
		}
	}()

	go func() { _, _ = io.Copy(p, ch) }()

	_, _ = io.Copy(ch, p)

	_ = ch.Close()
}

func (s *Servant) forwardSocks5(newChan ssh.NewChannel) {
	ch, _, err := newChan.Accept()
	if err != nil {
		s.Logger.Error("Failed to accept socks5 channel", "err", err)
		return
	}

	tunnel, err := yamux.Server(ch, nil)
	if err != nil {
		s.Logger.Error("Failed to create yamux session", slog.Any("err", err))
		return
	}

	proxy := socks5.NewServer()

	for {
		stream, err := tunnel.AcceptStream()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}

			s.Logger.Error("Failed to accept stream", slog.Any("err", err))
			return
		}

		go func() {
			_ = proxy.ServeConn(stream)
		}()
	}
}

func (s *Servant) shareDir(newChan ssh.NewChannel) {
	var meta MountDirMeta
	err := json.Unmarshal(newChan.ExtraData(), &meta)
	if err != nil {
		_ = newChan.Reject(UnmarshalMetaFailed, err.Error())
		return
	}

	ch, _, err := newChan.Accept()
	if err != nil {
		s.Logger.Error("Failed to accept ShareDir channel", "err", err)
		return
	}

	tunnel, err := yamux.Server(ch, nil)
	if err != nil {
		s.Logger.Error("Failed to create yamux session", "err", err)
	}

	if _, err := os.Stat(meta.Path); os.IsNotExist(err) {
		s.Logger.Error("remote directory does not exist", "path", meta.Path, "err", err)
	}

	bfs := osfs.New(meta.Path)
	bfsPlusChange := osfsx.New(bfs)

	handler := nfshelper.NewNullAuthHandler(bfsPlusChange)
	cacheHelper := nfshelper.NewCachingHandler(handler, meta.CacheLimit)

	nfs.Log.SetLevel(-1) // disable log
	err = nfs.Serve(tunnel, cacheHelper)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return
		}

		s.Logger.Error("failed to serve nfs", "err", err)
	}
}

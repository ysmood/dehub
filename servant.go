package dehub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/creack/pty"
	"github.com/hashicorp/yamux"
	"github.com/things-go/go-socks5"
	"golang.org/x/crypto/ssh"
)

func (n ServantName) String() string {
	return string(n)
}

func NewServant(logHandler slog.Handler, name ServantName, pubKeys []string) *Servant {
	return &Servant{
		l:       slog.New(logHandler),
		name:    name,
		pubKeys: pubKeys,
	}
}

func (s *Servant) Handle(conn net.Conn) func() {
	err := connectHub(conn, ClientTypeServant, s.name)
	if err != nil {
		s.l.Error("Failed to connect to hub", slog.Any("err", err))
		return func() {}
	}

	server, err := yamux.Server(conn, nil)
	if err != nil {
		s.l.Error("Failed to create yamux server", slog.Any("err", err))
		return func() {}
	}

	s.l.Info("servant connected to hub", slog.String("servant", s.name.String()))

	return func() {
		for {
			conn, err := server.Accept()
			if err != nil {
				s.l.Error("Failed to accept connection", slog.Any("err", err))
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

	var t time.Time

	err = json.Unmarshal(header.Timestamp, &t)
	if err != nil {
		return fmt.Errorf("invalid timestamp in header: %w", err)
	}

	if time.Since(t) > 10*time.Second {
		return errors.New("header has expired")
	}

	if !s.auth(header) {
		return errors.New("not authorized")
	}

	return s.serve(header, conn)
}

func (s *Servant) auth(header *TunnelHeader) bool {
	for _, raw := range s.pubKeys {
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
		if err != nil {
			s.l.Error("failed to parse public key", slog.Any("err", err))
			return false
		}

		err = pubKey.Verify(header.Timestamp, header.Sign)
		if err != nil {
			s.l.Error("invalid signature", slog.Any("err", err))
		} else {
			s.l.Info("authorized", slog.String("key", raw))
			return true
		}
	}

	return false
}

func (s *Servant) serve(h *TunnelHeader, conn net.Conn) error {
	s.l.Info("master connected", slog.Any("command", h.Command))

	switch h.Command {
	case CommandExec:
		return s.exec(conn, h.ExecMeta)
	case CommandForwardSocks5:
		return s.forwardSocks5(conn)
	case CommandForwardDir:
		return s.forwardDir(conn, h.ForwardDirMeta.Path)
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
		s.l.Error("Failed to create yamux session", slog.Any("err", err))
		return nil
	}

	proxy := socks5.NewServer()

	for {
		stream, err := tunnel.AcceptStream()
		if err != nil {
			s.l.Error("Failed to accept stream", slog.Any("err", err))
			return nil
		}

		go func() {
			_ = proxy.ServeConn(stream)
		}()
	}
}

func (s *Servant) forwardDir(conn net.Conn, path string) error {
	err := os.MkdirAll(path, 0o755) //nolint: gomnd
	if err != nil {
		return fmt.Errorf("failed to create mount directory: %w", err)
	}

	list, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	if len(list) > 0 {
		return fmt.Errorf("mount to non-empty dir is not allowed: %s", path)
	}

	fServer, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to create file server: %w", err)
	}

	defer func() { _ = fServer.Close() }()

	startTunnel(conn)

	tunnel, err := yamux.Client(conn, nil)
	if err != nil {
		s.l.Error("Failed to create yamux session", slog.Any("err", err))
		return nil
	}

	s.serveNFS(tunnel, fServer)

	port := strconv.Itoa(fServer.Addr().(*net.TCPAddr).Port)

	_ = exec.Command("umount", "-f", path).Run()

	err = exec.Command("mount",
		"-o", fmt.Sprintf("port=%s,mountport=%s", port, port),
		"-t", "nfs",
		"127.0.0.1:",
		path,
	).Run()
	if err != nil {
		s.l.Error("Failed to mount nfs", slog.Any("err", err))
		return nil
	}

	s.l.Info("nfs mounted", slog.String("path", path))

	<-tunnel.CloseChan()

	s.l.Info("nfs tunnel closed", slog.String("path", path))

	out, _ := exec.Command("umount", "-f", path).CombinedOutput()

	s.l.Info("nfs unmounted", slog.String("path", path), slog.String("out", string(out)))

	return nil
}

func (s *Servant) serveNFS(tunnel *yamux.Session, fServer net.Listener) {
	go func() {
		for {
			fConn, err := fServer.Accept()
			if err != nil {
				s.l.Error("Failed to accept connection", slog.Any("err", err))
				return
			}

			nfs, err := tunnel.Open()
			if err != nil {
				s.l.Error("Failed to open yamux stream", slog.Any("err", err))
				return
			}

			go func() {
				go func() {
					_, _ = io.Copy(nfs, fConn)
					_ = nfs.Close()
				}()

				_, _ = io.Copy(fConn, nfs)
				_ = fConn.Close()
			}()
		}
	}()
}

package dehub

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/creack/pty"
	"github.com/hashicorp/yamux"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func NewMaster(id ServantID, privateKey []byte) *Master {
	return &Master{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		servantID: id,
		prvKey:    privateKey,
	}
}

func (m *Master) Exec(conn net.Conn, in io.Reader, out io.Writer, cmd string, args ...string) error {
	err := connectHub(conn, ClientTypeMaster, m.servantID)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}

	header := &TunnelHeader{Command: CommandExec}

	size := &pty.Winsize{Rows: 24, Cols: 80} //nolint: gomnd

	if stdin, ok := in.(*os.File); ok && term.IsTerminal(int(stdin.Fd())) {
		size, err = pty.GetsizeFull(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to get terminal size: %w", err)
		}

		oldState, err := term.MakeRaw(int(stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to make raw terminal: %w", err)
		}

		defer func() { _ = term.Restore(int(stdin.Fd()), oldState) }()
	}

	header.ExecMeta = &ExecMeta{
		Size: size,
		Cmd:  cmd,
		Args: args,
	}

	err = m.handshake(conn, header)
	if err != nil {
		return fmt.Errorf("failed to handshake: %w", err)
	}

	go func() { _, _ = io.Copy(conn, in) }()

	_, _ = io.Copy(out, conn)

	_ = conn.Close()

	return nil
}

func (m *Master) ForwardSocks5(conn net.Conn, listenTo net.Listener) error {
	err := connectHub(conn, ClientTypeMaster, m.servantID)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}

	header := &TunnelHeader{
		Command: CommandForwardSocks5,
	}

	err = m.handshake(conn, header)
	if err != nil {
		return fmt.Errorf("failed to handshake: %w", err)
	}

	tunnel, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("failed to create yamux tunnel: %w", err)
	}

	for {
		src, err := listenTo.Accept()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("failed to accept sock5 connection: %w", err)
		}

		m.Logger.Info("new socks5 connection")

		go func() {
			stream, err := tunnel.Open()
			if err != nil {
				if errors.Is(err, yamux.ErrSessionShutdown) {
					return
				}

				m.Logger.Error("failed to open yamux tunnel", "err", err.Error())
				return
			}

			go func() {
				_, err := io.Copy(stream, src)
				if err != nil {
					m.Logger.Error(err.Error())
				}
				_ = stream.Close()
			}()

			_, err = io.Copy(src, stream)
			if err != nil {
				m.Logger.Error(err.Error())
			}
			_ = src.Close()
		}()
	}
}

func (m *Master) ServeNFS(conn net.Conn, remoteDir string, fsSrv net.Listener, cacheLimit int) error {
	if cacheLimit <= 0 {
		cacheLimit = 2048
	}

	err := connectHub(conn, ClientTypeMaster, m.servantID)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}

	header := &TunnelHeader{
		Command: CommandShareDir,
		ShareDirMeta: &MountDirMeta{
			Path:       remoteDir,
			CacheLimit: cacheLimit,
		},
	}

	err = m.handshake(conn, header)
	if err != nil {
		return fmt.Errorf("failed to handshake: %w", err)
	}

	tunnel, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("failed to create yamux session: %w", err)
	}

	m.serveNFS(tunnel, fsSrv)

	<-tunnel.CloseChan()

	return nil
}

func (m *Master) handshake(conn net.Conn, header *TunnelHeader) error {
	err := header.sign(m.prvKey)
	if err != nil {
		return fmt.Errorf("failed to sign header: %w", err)
	}

	writeMsg(conn, header)

	res, err := readMsg[string](conn)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if *res != "" {
		return fmt.Errorf("failed to handshake: %s", *res)
	}

	return nil
}

func (h *TunnelHeader) sign(keyData []byte) error {
	key, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	hash, err := publicKeyHash(key.PublicKey())
	if err != nil {
		return err
	}

	h.PubKeyHash = hash

	t, err := json.Marshal(time.Now())
	if err != nil {
		return fmt.Errorf("failed to marshal timestamp: %w", err)
	}

	h.Timestamp = t

	sig, err := key.Sign(rand.Reader, t)
	if err != nil {
		return fmt.Errorf("failed to sign timestamp: %w", err)
	}

	h.Sign = sig

	return nil
}

func (m *Master) serveNFS(tunnel *yamux.Session, fServer net.Listener) {
	go func() {
		for {
			fConn, err := fServer.Accept()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}

				m.Logger.Error("failed to accept nfs connection", slog.Any("err", err))
				return
			}

			nfs, err := tunnel.Open()
			if err != nil {
				if errors.Is(err, yamux.ErrSessionShutdown) {
					return
				}

				m.Logger.Error("failed to open nfs yamux stream", slog.Any("err", err))
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

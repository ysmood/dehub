package dehub

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/creack/pty"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/hashicorp/yamux"
	"github.com/willscott/go-nfs"
	nfshelper "github.com/willscott/go-nfs/helpers"
	osfsx "github.com/ysmood/dehub/lib/osfs"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func NewMaster(logHandler slog.Handler, name ServantName, privateKey []byte) *Master {
	return &Master{
		l:      slog.New(logHandler),
		name:   name,
		prvKey: privateKey,
	}
}

func (m *Master) Exec(conn net.Conn, in io.Reader, out io.Writer, cmd string, args ...string) error {
	err := connectHub(conn, ClientTypeMaster, m.name)
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
	err := connectHub(conn, ClientTypeMaster, m.name)
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
			return fmt.Errorf("failed to accept sock5 connection: %w", err)
		}

		m.l.Info("new socks5 connection")

		go func() {
			stream, err := tunnel.Open()
			if err != nil {
				m.l.Error("failed to open yamux tunnel", "err", err.Error())
				return
			}

			go func() {
				_, err := io.Copy(stream, src)
				if err != nil {
					m.l.Error(err.Error())
				}
				_ = stream.Close()
			}()

			_, err = io.Copy(src, stream)
			if err != nil {
				m.l.Error(err.Error())
			}
			_ = src.Close()
		}()
	}
}

func (m *Master) ForwardDir(conn net.Conn, localDir, remoteDir string, cacheLimit int) error {
	if cacheLimit <= 0 {
		cacheLimit = 2048
	}

	err := connectHub(conn, ClientTypeMaster, m.name)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}

	header := &TunnelHeader{
		Command:        CommandForwardDir,
		ForwardDirMeta: &ForwardDirMeta{Path: remoteDir},
	}

	err = m.handshake(conn, header)
	if err != nil {
		return fmt.Errorf("failed to handshake: %w", err)
	}

	tunnel, err := yamux.Server(conn, nil)
	if err != nil {
		return fmt.Errorf("failed to create yamux tunnel: %w", err)
	}

	if _, err := os.Stat(localDir); os.IsNotExist(err) {
		return fmt.Errorf("local directory does not exist: %w", err)
	}

	bfs := osfs.New(localDir)
	bfsPlusChange := osfsx.New(bfs)

	handler := nfshelper.NewNullAuthHandler(bfsPlusChange)
	cacheHelper := nfshelper.NewCachingHandler(handler, cacheLimit)

	nfs.Log.SetLevel(-1) // disable log
	err = nfs.Serve(tunnel, cacheHelper)
	if err != nil {
		return fmt.Errorf("failed to serve nfs: %w", err)
	}

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

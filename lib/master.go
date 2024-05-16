package dehub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/creack/pty"
	"github.com/elazarl/goproxy"
	"github.com/hashicorp/yamux"
	grpc_proxy "github.com/ysmood/grpc-tools/grpc-proxy"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/net/proxy"
	"golang.org/x/term"
)

func (c Command) String() string {
	return string(c)
}

// NewMaster creates a new master instance.
// If the prvKey is nil, it will try ssh agent to use the private key.
func NewMaster(id ServantID, prvKey ssh.Signer, check func(ssh.PublicKey) bool) *Master {
	authMethods := []ssh.AuthMethod{}
	if prvKey == nil {
		agentConn, _ := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
		agentClient := agent.NewClient(agentConn)
		authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
	} else {
		authMethods = append(authMethods, ssh.PublicKeys(prvKey))
	}

	sshConf := &ssh.ClientConfig{
		User: "user",
		Auth: authMethods,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			if check(key) {
				return nil
			}

			return fmt.Errorf("not trusted host pubkey %s, %v, %s", hostname, remote, ssh.FingerprintSHA256(key))
		},
	}

	return &Master{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		servantID: id,
		sshConf:   sshConf,
	}
}

// Connect to hub server.
func (m *Master) Connect(conn io.ReadWriteCloser) error {
	err := connectHub(conn, ClientTypeMaster, m.servantID)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}

	// This extra tunnel wrapping is for better control of the connection.
	// Such as timeout.
	session, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("failed to create master yamux session: %w", err)
	}

	tunnel, err := session.Open()
	if err != nil {
		return fmt.Errorf("failed to open master yamux tunnel: %w", err)
	}

	sshConn, _, _, err := ssh.NewClientConn(tunnel, "", m.sshConf)
	if err != nil {
		return fmt.Errorf("failed to create ssh client conn: %w", err)
	}

	m.sshConn = sshConn

	return nil
}

func (m *Master) Exec(in io.Reader, out io.Writer, cmd string, args ...string) error {
	size := &pty.Winsize{Rows: 24, Cols: 80} //nolint: mnd

	if stdin, ok := in.(*os.File); ok && term.IsTerminal(int(stdin.Fd())) {
		var err error
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

	meta, err := json.Marshal(ExecMeta{
		Size: size,
		Cmd:  cmd,
		Args: args,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal ExecMeta: %w", err)
	}

	ch, _, err := m.sshConn.OpenChannel(CommandExec.String(), meta)
	if err != nil {
		return fmt.Errorf("failed to open exec channel: %w", err)
	}

	defer func() { _ = ch.Close() }()

	defer m.sendWindowSizeChangeEvent(ch)()

	go func() { _, _ = io.Copy(ch, in) }()

	_, _ = io.Copy(out, ch)

	return nil
}

func (m *Master) ForwardSocks5(listenTo net.Listener) error {
	ch, _, err := m.sshConn.OpenChannel(CommandForwardSocks5.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to open socks5 channel: %w", err)
	}

	defer func() { _ = ch.Close() }()

	tunnel, err := yamux.Client(ch, nil)
	if err != nil {
		return fmt.Errorf("failed to create socks5 yamux tunnel: %w", err)
	}

	for {
		src, err := listenTo.Accept()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("failed to accept socks5 connection: %w", err)
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

func (m *Master) ForwardHTTP(listenTo net.Listener) error {
	ch, _, err := m.sshConn.OpenChannel(CommandForwardSocks5.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to open http proxy channel: %w", err)
	}

	defer func() { _ = ch.Close() }()

	tunnel, err := yamux.Client(ch, nil)
	if err != nil {
		return fmt.Errorf("failed to create http proxy yamux tunnel: %w", err)
	}

	dialer, _ := proxy.SOCKS5("tcp", "", nil, &tunnelDialer{tunnel})

	httpProxy := goproxy.NewProxyHttpServer()
	httpProxy.Tr.DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}

	return http.Serve(listenTo, httpProxy)
}

func (m *Master) ForwardGRPC(listenTo net.Listener) error {
	ch, _, err := m.sshConn.OpenChannel(CommandForwardSocks5.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to open http proxy channel: %w", err)
	}

	defer func() { _ = ch.Close() }()

	tunnel, err := yamux.Client(ch, nil)
	if err != nil {
		return fmt.Errorf("failed to create http proxy yamux tunnel: %w", err)
	}

	dialer, _ := proxy.SOCKS5("tcp", "", nil, &tunnelDialer{tunnel})

	proxyServer, _ := grpc_proxy.New(
		grpc_proxy.WithDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return dialer.Dial("tcp", s)
		}),
	)

	return proxyServer.Start(listenTo)
}

func (m *Master) ServeNFS(remoteDir string, fsSrv net.Listener, cacheLimit int) error {
	if cacheLimit <= 0 {
		cacheLimit = 2048
	}

	meta, err := json.Marshal(MountDirMeta{
		Path:       remoteDir,
		CacheLimit: cacheLimit,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal MountDirMeta: %w", err)
	}

	ch, _, err := m.sshConn.OpenChannel(CommandShareDir.String(), meta)
	if err != nil {
		return fmt.Errorf("failed to open ShareDir channel: %w", err)
	}

	defer func() { _ = ch.Close() }()

	tunnel, err := yamux.Client(ch, nil)
	if err != nil {
		return fmt.Errorf("failed to create nfs yamux session: %w", err)
	}

	m.serveNFS(tunnel, fsSrv)

	<-tunnel.CloseChan()

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

type tunnelDialer struct {
	tunnel interface {
		Open() (net.Conn, error)
	}
}

func (d *tunnelDialer) Dial(network, address string) (net.Conn, error) {
	return d.tunnel.Open()
}

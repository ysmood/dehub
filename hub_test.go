package dehub_test

import (
	"bytes"
	"net"
	"net/http"
	"testing"

	"github.com/willscott/go-nfs-client/nfs"
	"github.com/willscott/go-nfs-client/nfs/rpc"
	"github.com/ysmood/dehub"
	"github.com/ysmood/got"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

func TestExec(t *testing.T) {
	g := got.T(t)

	hub := dehub.NewHub()
	hub.GetIP = func() (string, error) {
		return "127.0.0.1", nil
	}

	servant := dehub.NewServant("test", prvKey(g), pubKey(g))

	master := dehub.NewMaster("test", prvKey(g), pubKey(g))

	hubSrv, err := net.Listen("tcp", ":0")
	g.E(err)

	go func() {
		for {
			conn, err := hubSrv.Accept()
			if err != nil {
				return
			}

			go hub.Handle(conn)
		}
	}()

	servantConn, err := net.Dial("tcp", hubSrv.Addr().String())
	g.E(err)

	go servant.Handle(servantConn)()

	masterConn, err := net.Dial("tcp", hubSrv.Addr().String())
	g.E(err)

	g.E(master.Connect(masterConn))

	txt := g.RandStr(1024)

	out := bytes.NewBuffer(nil)
	err = master.Exec(bytes.NewBuffer(nil), out, "echo", txt)
	g.E(err)
	g.Has(out.String(), txt)

	proxyServer, err := net.Listen("tcp", ":0")
	g.E(err)

	go func() { g.E(master.ForwardSocks5(proxyServer)) }()

	res := reqViaProxy(g, proxyServer.Addr().String(), "http://example.com")
	g.Has(res, "Example Domain")
}

func reqViaProxy(g got.G, proxyHost string, u string) string {
	dialer, err := proxy.SOCKS5("tcp", proxyHost, nil, proxy.Direct)
	g.E(err)

	client := &http.Client{Transport: &http.Transport{Dial: dialer.Dial}}

	req, err := http.NewRequestWithContext(g.Context(), "", u, nil)
	g.E(err)

	res, err := client.Do(req)
	g.E(err)
	defer func() { g.E(res.Body.Close()) }()

	return g.Read(res.Body).String()
}

func TestMountDir(t *testing.T) {
	g := got.T(t)

	hub := dehub.NewHub()
	hub.GetIP = func() (string, error) {
		return "127.0.0.1", nil
	}

	servant := dehub.NewServant("test", prvKey(g), pubKey(g))

	master := dehub.NewMaster("test", prvKey(g), pubKey(g))

	hubSrv, err := net.Listen("tcp", ":0")
	g.E(err)

	go func() {
		for {
			conn, err := hubSrv.Accept()
			if err != nil {
				return
			}

			go hub.Handle(conn)
		}
	}()

	servantConn, err := net.Dial("tcp", hubSrv.Addr().String())
	g.E(err)

	go servant.Handle(servantConn)()

	masterConn, err := net.Dial("tcp", hubSrv.Addr().String())
	g.E(err)

	g.E(master.Connect(masterConn))

	fsSrv, err := net.Listen("tcp", ":0")
	g.E(err)

	go func() { g.E(master.ServeNFS("lib/fixtures", fsSrv, 0)) }()

	g.Eq(
		g.Read("lib/fixtures/id_ed25519.pub").String(),
		nfsReadFile(g, fsSrv.Addr().(*net.TCPAddr), "id_ed25519.pub"),
	)
}

func nfsReadFile(g got.G, addr *net.TCPAddr, path string) string {
	c, err := rpc.DialTCP("tcp", addr.String(), false)
	g.E(err)
	defer c.Close()

	var mounter nfs.Mount
	mounter.Client = c
	target, err := mounter.Mount("/", rpc.AuthNull)
	g.E(err)
	defer func() {
		_ = mounter.Unmount()
	}()

	f, err := target.Open(path)
	g.E(err)

	return g.Read(f).String()
}

func prvKey(g got.G) ssh.Signer {
	key, err := ssh.ParsePrivateKey(g.Read("lib/fixtures/id_ed25519").Bytes())
	g.E(err)
	return key
}

func pubKey(g got.G) []byte {
	return g.Read("lib/fixtures/id_ed25519.pub").Bytes()
}

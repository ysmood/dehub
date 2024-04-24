package dehub_test

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ysmood/dehub"
	"github.com/ysmood/got"
	"golang.org/x/net/proxy"
)

func TestBasic(t *testing.T) {
	g := got.T(t)

	log := slog.NewJSONHandler(io.Discard, nil)

	hub := dehub.NewHub(log)
	hub.GetIP = func() (string, error) {
		return "127.0.0.1", nil
	}

	servant := dehub.NewServant(log, "test", []string{g.Read("lib/fixtures/id_ed25519.pub").String()})

	master := dehub.NewMaster(log, "test", g.Read("lib/fixtures/id_ed25519").Bytes())

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

	txt := g.RandStr(1024)

	out := bytes.NewBuffer(nil)
	err = master.Exec(masterConn, bytes.NewBuffer(nil), out, "echo", txt)
	g.E(err)
	g.Has(out.String(), txt)

	proxy, err := net.Listen("tcp", ":0")
	g.E(err)

	masterConn, err = net.Dial("tcp", hubSrv.Addr().String())
	g.E(err)

	go func() { g.E(master.ForwardSocks5(masterConn, proxy)) }()

	res := getWithProxy(g, proxy.Addr().String(), "http://example.com")
	g.Has(res, "Example Domain")

	masterConn, err = net.Dial("tcp", hubSrv.Addr().String())
	g.E(err)
}

func getWithProxy(g got.G, proxyHost string, u string) string {
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

func TestForwardDir(t *testing.T) {
	g := got.T(t)

	log := slog.NewJSONHandler(io.Discard, nil)

	hub := dehub.NewHub(log)
	hub.GetIP = func() (string, error) {
		return "127.0.0.1", nil
	}

	servant := dehub.NewServant(log, "test", []string{g.Read("lib/fixtures/id_ed25519.pub").String()})

	master := dehub.NewMaster(log, "test", g.Read("lib/fixtures/id_ed25519").Bytes())

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

	dir, err := os.MkdirTemp("", "dehub-nfs")
	g.E(err)

	go func() { g.E(master.ForwardDir(masterConn, "lib/fixtures", dir, 0)) }()

	time.Sleep(time.Second)

	g.True(g.PathExists(dir + "/id_ed25519.pub"))
}

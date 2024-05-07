package dehub_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/willscott/go-nfs-client/nfs"
	"github.com/willscott/go-nfs-client/nfs/rpc"
	dehub "github.com/ysmood/dehub/lib"
	"github.com/ysmood/dehub/lib/hubdb"
	"github.com/ysmood/got"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

func TestExec(t *testing.T) {
	g := got.T(t)

	hubAddr := startHub(g, nil)

	servantConn, err := net.Dial("tcp", hubAddr)
	g.E(err)
	servant := dehub.NewServant("test", prvKey(g), pubKey(g))
	go servant.Serve(servantConn)()

	masterConn, err := net.Dial("tcp", hubAddr)
	g.E(err)
	master := dehub.NewMaster("test", prvKey(g), pubKey(g))
	g.E(master.Connect(masterConn))

	txt := g.RandStr(1024)

	out := bytes.NewBuffer(nil)
	err = master.Exec(bytes.NewBuffer(nil), out, "echo", txt)
	g.E(err)
	g.Has(out.String(), txt)
}

func TestSocks5(t *testing.T) {
	g := got.T(t)

	hubAddr := startHub(g, nil)

	servantConn, err := net.Dial("tcp", hubAddr)
	g.E(err)
	servant := dehub.NewServant("test", prvKey(g), pubKey(g))
	go servant.Serve(servantConn)()

	masterConn, err := net.Dial("tcp", hubAddr)
	g.E(err)
	master := dehub.NewMaster("test", prvKey(g), pubKey(g))
	g.E(master.Connect(masterConn))

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

	hubAddr := startHub(g, nil)

	servantConn, err := net.Dial("tcp", hubAddr)
	g.E(err)
	servant := dehub.NewServant("test", prvKey(g), pubKey(g))
	go servant.Serve(servantConn)()

	masterConn, err := net.Dial("tcp", hubAddr)
	g.E(err)
	master := dehub.NewMaster("test", prvKey(g), pubKey(g))
	g.E(master.Connect(masterConn))

	fsSrv, err := net.Listen("tcp", ":0")
	g.E(err)

	go func() { g.E(master.ServeNFS("fixtures", fsSrv, 0)) }()

	g.Eq(
		g.Read("fixtures/id_ed25519.pub").String(),
		nfsReadFile(g, fsSrv.Addr().(*net.TCPAddr), "id_ed25519.pub"),
	)
}

func TestCluster(t *testing.T) {
	g := got.T(t)

	client, err := mongo.Connect(
		context.Background(),
		options.Client().ApplyURI("mongodb://localhost:27017"),
	)
	g.E(err)

	db := hubdb.NewMongo(client.Database("test"), "test")

	hub01Addr := startHub(g, db)
	hub02Addr := startHub(g, db)

	startServant := func(name dehub.ServantID, hubAddr string) {
		servantConn, err := net.Dial("tcp", hubAddr)
		g.E(err)

		servant := dehub.NewServant(name, prvKey(g), pubKey(g))

		go servant.Serve(servantConn)()
	}

	startServant("test01", hub01Addr)
	startServant("test02", hub02Addr)

	time.Sleep(time.Second)

	testMaster := func(hubAddr string) {
		masterConn, err := net.Dial("tcp", hubAddr)
		g.E(err)

		master := dehub.NewMaster("test01", prvKey(g), pubKey(g))
		g.E(master.Connect(masterConn))

		txt := g.RandStr(1024)

		out := bytes.NewBuffer(nil)
		err = master.Exec(bytes.NewBuffer(nil), out, "echo", txt)
		g.E(err)
		g.Has(out.String(), txt)
	}

	testMaster(hub01Addr)
	testMaster(hub02Addr)
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
	key, err := ssh.ParsePrivateKey(g.Read("fixtures/id_ed25519").Bytes())
	g.E(err)
	return key
}

func pubKey(g got.G) []byte {
	return g.Read("fixtures/id_ed25519.pub").Bytes()
}

func getIP() (string, error) {
	return "127.0.0.1", nil
}

func startHub(g got.G, db dehub.DB) string {
	hubSrv, err := net.Listen("tcp", ":0")
	g.E(err)

	if db == nil {
		db = hubdb.NewMemory()
	}

	hub := dehub.NewHub()
	hub.GetIP = getIP
	hub.DB = db

	go func() {
		for {
			conn, err := hubSrv.Accept()
			if err != nil {
				return
			}

			go hub.Handle(conn)
		}
	}()

	return hubSrv.Addr().String()
}

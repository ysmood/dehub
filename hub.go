package dehub

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/hashicorp/yamux"
	"github.com/ysmood/dehub/lib/xsync"
	"github.com/ysmood/myip"
)

func NewHub(logHandler slog.Handler) *Hub {
	h := &Hub{
		l:    slog.New(logHandler),
		list: xsync.Map[ServantName, *yamux.Session]{},
		db: &memDB{
			list: xsync.Map[ServantName, string]{},
		},
		addr: "",
		GetIP: func() (string, error) {
			return myip.New().GetInterfaceIP()
		},
	}

	h.startRelay()

	return h
}

func connectHub(conn net.Conn, typ ClientType, name ServantName) error {
	writeMsg(conn, &HubHeader{
		Type: typ,
		Name: name,
	})

	res, err := readMsg[string](conn)
	if err != nil {
		return fmt.Errorf("failed to read ack: %w", err)
	}

	if *res != "" {
		return fmt.Errorf("hub response error: %s", *res)
	}

	return nil
}

func (h *Hub) Handle(conn net.Conn) {
	header, err := readMsg[HubHeader](conn)
	if err != nil {
		h.l.Error("Failed to read header", slog.Any("err", err))
		writeMsg(conn, "Failed to read header: "+err.Error())
		return
	}

	startTunnel(conn)

	switch header.Type {
	case ClientTypeServant:
		err = h.handleServant(conn, header)

	case ClientTypeMaster:
		err = h.handleMaster(conn, header)
	}

	if err != nil {
		h.l.Error("Failed to handle", slog.Any("err", err))

		_ = conn.Close()
	}
}

func (h *Hub) handleServant(conn net.Conn, header *HubHeader) error {
	tunnel, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("failed to create yamux session: %w", err)
	}

	h.list.Store(header.Name, tunnel)

	err = h.db.StoreLocation(header.Name, h.addr)
	if err != nil {
		return fmt.Errorf("failed to store location: %w", err)
	}

	h.l.Info("servant connected", slog.String("name", header.Name.String()))

	<-tunnel.CloseChan()

	h.l.Info("servant disconnected", slog.String("name", header.Name.String()))

	h.list.Delete(header.Name)

	_ = conn.Close()

	return nil
}

func (h *Hub) handleMaster(conn net.Conn, header *HubHeader) error {
	defer func() { _ = conn.Close() }()

	addr, err := h.db.LoadLocation(header.Name)
	if err != nil {
		return fmt.Errorf("failed to load location: %w", err)
	}

	relay, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial relay: %w", err)
	}

	defer func() { _ = relay.Close() }()

	writeMsg(relay, header.Name)

	res, err := readMsg[string](relay)
	if err != nil {
		return fmt.Errorf("failed to read relay ack: %w", err)
	}

	if *res != "" {
		return fmt.Errorf("relay response error: %s", *res)
	}

	h.l.Info("master connected to hub", slog.String("name", header.Name.String()))

	go func() {
		_, _ = io.Copy(relay, conn)
		_ = relay.Close()
	}()

	_, _ = io.Copy(conn, relay)

	h.l.Info("master disconnected", slog.String("name", header.Name.String()))

	return nil
}

func (h *Hub) startRelay() {
	relay, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	addr, err := h.GetIP()
	if err != nil {
		panic(err)
	}

	h.addr = net.JoinHostPort(addr, strconv.Itoa(relay.Addr().(*net.TCPAddr).Port))

	go func() {
		for {
			conn, err := relay.Accept()
			if err != nil {
				h.l.Error("failed to accept", slog.Any("err", err))

				return
			}

			go func() {
				defer func() { _ = conn.Close() }()

				err = h.handleRelay(conn)
				if err != nil {
					h.l.Error("failed to handle relay", slog.Any("err", err))
					writeMsg(conn, err.Error())
					return
				}

				h.l.Info("relay disconnected")
			}()
		}
	}()
}

func (h *Hub) handleRelay(conn net.Conn) error {
	name, err := readMsg[ServantName](conn)
	if err != nil {
		return fmt.Errorf("failed to read servant name: %w", err)
	}

	var servant *yamux.Session

	h.list.Range(func(key ServantName, value *yamux.Session) bool {
		if strings.HasPrefix(key.String(), name.String()) {
			servant = value

			return false
		}

		return true
	})

	if servant == nil {
		return fmt.Errorf("servant not found: %s", name.String())
	}

	startTunnel(conn)

	h.l.Info("relay connected", slog.String("name", name.String()))

	tunnel, err := servant.Open()
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	defer func() { _ = tunnel.Close() }()

	go func() {
		_, _ = io.Copy(tunnel, conn)
		_ = tunnel.Close()
	}()

	_, _ = io.Copy(conn, tunnel)

	return nil
}

type memDB struct {
	list xsync.Map[ServantName, string]
}

func (db *memDB) StoreLocation(name ServantName, addr string) error {
	db.list.Store(name, addr)

	return nil
}

func (db *memDB) LoadLocation(name ServantName) (string, error) {
	addr, ok := db.list.Load(name)
	if !ok {
		return "", fmt.Errorf("servant not found: %s", name.String())
	}

	return addr, nil
}

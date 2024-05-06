package dehub

import (
	"errors"
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

func NewHub() *Hub {
	h := &Hub{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		list:   xsync.Map[ServantID, *yamux.Session]{},
		DB: &memDB{
			list: xsync.Map[ServantID, string]{},
		},
		addr: "",
		GetIP: func() (string, error) {
			return myip.New().GetInterfaceIP()
		},
	}

	h.startRelay()

	return h
}

func connectHub(conn io.ReadWriter, typ ClientType, name ServantID) error {
	writeMsg(conn, &HubHeader{
		Type: typ,
		ID:   name,
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

func (h *Hub) Handle(conn io.ReadWriteCloser) {
	header, err := readMsg[HubHeader](conn)
	if err != nil {
		h.Logger.Error("Failed to read header", slog.Any("err", err))
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
		h.Logger.Error("Failed to handle", slog.Any("err", err))

		_ = conn.Close()
	}
}

func (h *Hub) handleServant(conn io.ReadWriteCloser, header *HubHeader) error {
	tunnel, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("failed to create yamux session: %w", err)
	}

	h.list.Store(header.ID, tunnel)

	err = h.DB.StoreLocation(header.ID, h.addr)
	if err != nil {
		return fmt.Errorf("failed to store location: %w", err)
	}

	h.Logger.Info("servant connected", slog.String("name", header.ID.String()))

	<-tunnel.CloseChan()

	h.Logger.Info("servant disconnected", slog.String("name", header.ID.String()))

	h.list.Delete(header.ID)

	_ = conn.Close()

	return nil
}

func (h *Hub) handleMaster(conn io.ReadWriteCloser, header *HubHeader) error {
	defer func() { _ = conn.Close() }()

	addr, err := h.DB.LoadLocation(header.ID)
	if err != nil {
		return fmt.Errorf("failed to load location: %w", err)
	}

	relay, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial relay: %w", err)
	}

	defer func() { _ = relay.Close() }()

	writeMsg(relay, header.ID)

	res, err := readMsg[string](relay)
	if err != nil {
		return fmt.Errorf("failed to read relay ack: %w", err)
	}

	if *res != "" {
		return fmt.Errorf("relay response error: %s", *res)
	}

	h.Logger.Info("master connected to hub", slog.String("name", header.ID.String()))

	go func() {
		_, _ = io.Copy(relay, conn)
		_ = relay.Close()
	}()

	_, _ = io.Copy(conn, relay)

	h.Logger.Info("master disconnected", slog.String("name", header.ID.String()))

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
				if errors.Is(err, io.EOF) {
					return
				}

				h.Logger.Error("failed to accept", slog.Any("err", err))

				return
			}

			go func() {
				defer func() { _ = conn.Close() }()

				err = h.handleRelay(conn)
				if err != nil {
					h.Logger.Error("failed to handle relay", slog.Any("err", err))
					writeMsg(conn, err.Error())
					return
				}

				h.Logger.Info("relay disconnected")
			}()
		}
	}()
}

func (h *Hub) handleRelay(conn net.Conn) error {
	name, err := readMsg[ServantID](conn)
	if err != nil {
		return fmt.Errorf("failed to read servant name: %w", err)
	}

	var servant *yamux.Session

	h.list.Range(func(key ServantID, value *yamux.Session) bool {
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

	h.Logger.Info("relay connected", slog.String("name", name.String()))

	tunnel, err := servant.Open()
	if err != nil {
		if errors.Is(err, yamux.ErrSessionShutdown) {
			return nil
		}

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
	list xsync.Map[ServantID, string]
}

func (db *memDB) StoreLocation(name ServantID, addr string) error {
	db.list.Store(name, addr)

	return nil
}

func (db *memDB) LoadLocation(name ServantID) (string, error) {
	addr, ok := db.list.Load(name)
	if !ok {
		return "", fmt.Errorf("servant not found: %s", name.String())
	}

	return addr, nil
}

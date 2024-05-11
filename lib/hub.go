package dehub

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/ysmood/dehub/lib/hubdb"
	"github.com/ysmood/dehub/lib/xsync"
	"github.com/ysmood/myip"
)

func NewHub() *Hub {
	h := &Hub{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		list:   xsync.Map[ServantID, *yamux.Session]{},
		DB:     hubdb.NewMemory(),
		addr:   "",
		GetIP: func() (string, error) {
			return myip.New().GetInterfaceIP()
		},
	}

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
	if h.addr == "" {
		writeMsg(conn, "relay server failed to start")
		return
	}

	header, err := readMsg[HubHeader](conn)
	if err != nil {
		h.Logger.Error("failed to read header", slog.Any("err", err))
		writeMsg(conn, "failed to read header: "+err.Error())
		return
	}

	switch header.Type {
	case ClientTypeServant:
		err = h.handleServant(conn, header)

	case ClientTypeMaster:
		err = h.handleMaster(conn, header)
	}

	if err != nil {
		writeMsg(conn, err.Error())
	}
}

func (h *Hub) handleServant(conn io.ReadWriteCloser, header *HubHeader) error {
	tunnel, err := yamux.Client(conn, nil)
	if err != nil {
		return fmt.Errorf("failed to create yamux session: %w", err)
	}

	h.list.Store(header.ID, tunnel)

	err = h.DB.StoreLocation(header.ID.String(), h.addr)
	if err != nil {
		return fmt.Errorf("failed to store location: %w", err)
	}

	go func() {
		for !tunnel.IsClosed() {
			time.Sleep(hubdb.HeartbeatInterval)
			_ = h.DB.StoreLocation(header.ID.String(), h.addr)
		}
	}()

	startTunnel(conn)

	h.Logger.Info("servant connected hub", slog.String("servantId", header.ID.String()))

	<-tunnel.CloseChan()

	h.Logger.Info("servant disconnected from hub", slog.String("servantId", header.ID.String()))

	h.list.Delete(header.ID)

	err = h.DB.DeleteLocation(header.ID.String())
	if err != nil {
		return fmt.Errorf("failed to delete location: %w", err)
	}

	_ = conn.Close()

	return nil
}

func (h *Hub) handleMaster(conn io.ReadWriteCloser, header *HubHeader) error {
	addr, id, err := h.DB.LoadLocation(header.ID.String())
	if err != nil {
		return fmt.Errorf("failed to get servant location: %w", err)
	}

	relay, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial relay: %w", err)
	}

	writeMsg(relay, id)

	res, err := readMsg[string](relay)
	if err != nil {
		return fmt.Errorf("failed to read relay ack: %w", err)
	}

	if *res != "" {
		return fmt.Errorf("relay response error: %s", *res)
	}

	startTunnel(conn)

	h.Logger.Info("master connected to hub", slog.String("name", header.ID.String()))

	go func() {
		_, _ = io.Copy(relay, conn)
		_ = relay.Close()
	}()

	_, _ = io.Copy(conn, relay)
	_ = conn.Close()

	h.Logger.Info("master disconnected", slog.String("name", header.ID.String()))

	return nil
}

// MustStartRelay is similar to [Hub.StartRelay].
func (h *Hub) MustStartRelay() func() {
	fn, err := h.StartRelay(":0")
	if err != nil {
		panic(err)
	}

	return fn
}

func (h *Hub) StartRelay(addr string) (func(), error) {
	relay, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen relay: %w", err)
	}

	ip, err := h.GetIP()
	if err != nil {
		return nil, fmt.Errorf("failed to get ip for relay: %w", err)
	}

	h.addr = net.JoinHostPort(ip, strconv.Itoa(relay.Addr().(*net.TCPAddr).Port))

	h.Logger.Info("relay server started", slog.String("addr", h.addr))

	return func() {
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
				err = h.handleRelay(conn)
				if err != nil {
					writeMsg(conn, err.Error())
				}

				h.Logger.Info("relay disconnected")
			}()
		}
	}, nil
}

func (h *Hub) handleRelay(conn net.Conn) error {
	id, err := readMsg[ServantID](conn)
	if err != nil {
		return fmt.Errorf("failed to read servant name: %w", err)
	}

	servant, has := h.list.Load(*id)
	if !has {
		_ = h.DB.DeleteLocation(id.String())
		return fmt.Errorf("servant not found: %s", id.String())
	}

	h.Logger.Info("relay connected", slog.String("name", id.String()))

	tunnel, err := servant.Open()
	if err != nil {
		if errors.Is(err, yamux.ErrSessionShutdown) {
			return nil
		}

		return fmt.Errorf("failed to open stream: %w", err)
	}

	startTunnel(conn)

	defer func() { _ = tunnel.Close() }()

	go func() {
		_, _ = io.Copy(tunnel, conn)
		_ = tunnel.Close()
	}()

	_, _ = io.Copy(conn, tunnel)
	_ = conn.Close()

	return nil
}

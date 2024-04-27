package dehub

import (
	"encoding/json"
	"net"

	"github.com/ysmood/byframe"
)

func startTunnel(conn net.Conn) {
	writeMsg(conn, "")
}

func writeMsg(conn net.Conn, msg any) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}

	_, _ = conn.Write(byframe.Encode(b))
}

func readMsg[T any](conn net.Conn) (*T, error) {
	s := byframe.NewScanner(conn)

	s.Scan()

	err := s.Err()
	if err != nil {
		return nil, err
	}

	var msg T

	err = json.Unmarshal(s.Frame(), &msg)
	if err != nil {
		return nil, err
	}

	return &msg, nil
}

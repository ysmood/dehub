package dehub

import (
	"encoding/json"
	"io"

	"github.com/ysmood/byframe"
)

func startTunnel(conn io.Writer) {
	writeMsg(conn, "")
}

func writeMsg(conn io.Writer, msg any) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}

	_, _ = conn.Write(byframe.Encode(b))
}

func readMsg[T any](conn io.Reader) (*T, error) {
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

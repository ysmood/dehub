package dehub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/gobwas/ws"
	"github.com/ysmood/byframe"
	"golang.org/x/crypto/ssh"
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

func WebsocketUpgrade(conn io.ReadWriter) error {
	_, err := ws.Upgrade(conn)
	return err
}

func WebsocketDial(ctx context.Context, addr string) (net.Conn, error) {
	conn, _, _, err := ws.Dial(ctx, addr)
	return conn, err
}

func CheckPublicKeys(trustedPubKeys ...[]byte) func(ssh.PublicKey) bool {
	trusted := map[string]struct{}{}

	for _, raw := range trustedPubKeys {
		key, _, _, _, err := ssh.ParseAuthorizedKey(raw)
		if err != nil {
			panic(err)
		}

		trusted[ssh.FingerprintSHA256(key)] = struct{}{}
	}

	return func(key ssh.PublicKey) bool {
		_, ok := trusted[ssh.FingerprintSHA256(key)]
		return ok
	}
}

func FormatPubKey(key ssh.PublicKey) string {
	return fmt.Sprintf("%s %s", key.Type(), base64.StdEncoding.EncodeToString(key.Marshal()))
}

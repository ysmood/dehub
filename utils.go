package dehub

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net"

	"github.com/ysmood/byframe"
	"golang.org/x/crypto/ssh"
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

func publicKeyHash(pub ssh.PublicKey) ([md5.Size]byte, error) {
	key, ok := pub.(ssh.CryptoPublicKey)
	if !ok {
		return [md5.Size]byte{}, fmt.Errorf("invalid public key type: %T", pub)
	}

	sshPubKey, err := ssh.NewPublicKey(key.CryptoPublicKey())
	if err != nil {
		return [md5.Size]byte{}, err
	}

	d := md5.Sum(sshPubKey.Marshal())

	return d, nil
}

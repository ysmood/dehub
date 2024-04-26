package dehub

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"

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

func MountNFS(addr *net.TCPAddr, localDir string) error {
	err := os.MkdirAll(localDir, 0o755) //nolint: gomnd
	if err != nil {
		return fmt.Errorf("failed to create mount directory: %w", err)
	}

	list, err := os.ReadDir(localDir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	if len(list) > 0 {
		return fmt.Errorf("mount to non-empty dir is not allowed: %s", localDir)
	}

	port := strconv.Itoa(addr.Port)

	_ = exec.Command("umount", "-f", localDir).Run()

	out, err := exec.Command("mount",
		"-o", fmt.Sprintf("port=%s,mountport=%s", port, port),
		"-t", "nfs",
		"127.0.0.1",
		localDir,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to mount nfs: %w: %s", err, out)
	}

	return nil
}

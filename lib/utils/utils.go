package utils

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
)

func MountNFS(addr *net.TCPAddr, localDir string) error {
	err := os.MkdirAll(localDir, 0o755) //nolint: mnd,gomnd
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
		"127.0.0.1:",
		localDir,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to mount nfs: %w: %s", err, out)
	}

	return nil
}

func UnmountNFS(localDir string) error {
	out, err := exec.Command("umount", "-f", localDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to unmount nfs: %w: %s", err, out)
	}

	return nil
}

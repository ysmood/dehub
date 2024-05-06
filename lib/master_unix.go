//go:build !windows

package dehub

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

func (m *Master) sendWindowSizeChangeEvent(ch ssh.Channel) func() {
	change := make(chan os.Signal, 1)
	signal.Notify(change, syscall.SIGWINCH)

	go func() {
		for range change {
			size, err := pty.GetsizeFull(os.Stdin)
			if err != nil {
				m.Logger.Error("failed to get terminal size", "err", err.Error())
				return
			}

			b, err := json.Marshal(size)
			if err != nil {
				m.Logger.Error("failed to marshal terminal size", "err", err.Error())
				return
			}

			_, err = ch.SendRequest(ExecResizeRequest, false, b)
			if err != nil {
				m.Logger.Error("failed to send resize request", "err", err.Error())
				return
			}
		}
	}()

	return func() { signal.Stop(change); close(change) }
}

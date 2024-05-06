//go:build windows

package dehub

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

func (m *Master) sendWindowSizeChangeEvent(ch ssh.Channel) func() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		for ctx.Err() == nil {
			time.Sleep(time.Second * 3)

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

	return cancel
}

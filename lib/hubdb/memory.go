package hubdb

import (
	"fmt"
	"strings"

	"github.com/ysmood/dehub/lib/xsync"
)

type Memory struct {
	list xsync.Map[string, string]
}

func NewMemory() *Memory {
	return &Memory{
		list: xsync.Map[string, string]{},
	}
}

func (db *Memory) StoreLocation(id string, addr string) error {
	db.list.Store(id, addr)

	return nil
}

func (db *Memory) LoadLocation(idPrefix string) (string, string, error) {
	var addr, locID string
	db.list.Range(func(id, value string) bool {
		if strings.HasPrefix(id, idPrefix) {
			locID = id
			addr = value
			return false
		}

		return true
	})

	if addr == "" {
		return "", "", fmt.Errorf("%w via id prefix: %s", ErrNotFound, idPrefix)
	}

	return addr, locID, nil
}

func (db *Memory) DeleteLocation(id string) error {
	db.list.Delete(id)
	return nil
}

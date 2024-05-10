package hubdb

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

// HeartbeatInterval is used to detect if a location is still alive.
const HeartbeatInterval = 30 * time.Second

const LocationExpiration = 2 * HeartbeatInterval

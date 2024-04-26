package dehub

import (
	"crypto/md5"
	"log/slog"
	"time"

	"github.com/creack/pty"
	"github.com/hashicorp/yamux"
	"github.com/ysmood/dehub/lib/xsync"
	"golang.org/x/crypto/ssh"
)

type ServantID string

type Hub struct {
	Logger *slog.Logger
	list   xsync.Map[ServantID, *yamux.Session]
	db     DB
	addr   string // The net address of the hub node relay.

	GetIP func() (string, error)
}

type ClientType int

const (
	ClientTypeServant ClientType = iota
	ClientTypeMaster
)

type HubHeader struct {
	Type ClientType
	ID   ServantID
}

// DB store the location of which hub node the servant is connected to.
type DB interface {
	StoreLocation(id ServantID, netAddr string) error
	LoadLocation(id ServantID) (netAddr string, err error)
}

type Master struct {
	Logger    *slog.Logger
	prvKey    []byte
	servantID ServantID
}

type Servant struct {
	Logger      *slog.Logger
	SignTimeout time.Duration
	pubKeys     PubKeys
	id          ServantID
}

type PubKeys map[[md5.Size]byte]PubKey

type PubKey struct {
	raw       string
	sshPubKey ssh.PublicKey
}

type TunnelHeader struct {
	Timestamp    []byte
	PubKeyHash   [md5.Size]byte
	Sign         *ssh.Signature
	Command      Command
	ExecMeta     *ExecMeta
	ShareDirMeta *MountDirMeta
}

type ExecMeta struct {
	Size *pty.Winsize
	Cmd  string
	Args []string
}

type MountDirMeta struct {
	Path       string
	CacheLimit int
}

type Command int

const (
	CommandExec Command = iota
	CommandForwardSocks5
	CommandShareDir
)

type Logger interface {
	Info(string, ...slog.Attr)
	Warn(string, ...slog.Attr)
	Error(string, ...slog.Attr)
}

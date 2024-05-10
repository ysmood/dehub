package dehub

import (
	"crypto/md5"
	"log/slog"

	"github.com/creack/pty"
	"github.com/hashicorp/yamux"
	"github.com/ysmood/dehub/lib/xsync"
	"golang.org/x/crypto/ssh"
)

type ServantID string

type Hub struct {
	Logger *slog.Logger
	list   xsync.Map[ServantID, *yamux.Session]
	DB     DB
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
	StoreLocation(id string, netAddr string) error
	LoadLocation(idPrefix string) (netAddr string, id string, err error)
	DeleteLocation(id string) error
}

type Master struct {
	Logger    *slog.Logger
	servantID ServantID
	sshConf   *ssh.ClientConfig
	sshConn   ssh.Conn
}

type Servant struct {
	Logger  *slog.Logger
	id      ServantID
	sshConf *ssh.ServerConfig
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

type Command string

const (
	CommandExec          Command = "exec"
	CommandForwardSocks5 Command = "forward-socks5"
	CommandShareDir      Command = "share-dir"
)

const ExecResizeRequest = "resize"

type Logger interface {
	Info(string, ...slog.Attr)
	Warn(string, ...slog.Attr)
	Error(string, ...slog.Attr)
}

const (
	UnmarshalMetaFailed ssh.RejectionReason = iota + ssh.ResourceShortage + 1000
	FailedStartPTY
)

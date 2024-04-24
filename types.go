package dehub

import (
	"log/slog"

	"github.com/creack/pty"
	"github.com/hashicorp/yamux"
	"github.com/ysmood/dehub/lib/xsync"
	"golang.org/x/crypto/ssh"
)

type ServantName string

type Hub struct {
	l    *slog.Logger
	list xsync.Map[ServantName, *yamux.Session]
	db   DB
	addr string // The net address of the hub node relay.

	GetIP func() (string, error)
}

type ClientType int

const (
	ClientTypeServant ClientType = iota
	ClientTypeMaster
)

type HubHeader struct {
	Type ClientType
	Name ServantName
}

// DB store the location of which hub node the servant is connected to.
type DB interface {
	StoreLocation(name ServantName, netAddr string) error
	LoadLocation(name ServantName) (netAddr string, err error)
}

type Master struct {
	l      *slog.Logger
	prvKey []byte
	name   ServantName
}

type Servant struct {
	l       *slog.Logger
	pubKeys []string
	name    ServantName
}

type TunnelHeader struct {
	Timestamp      []byte
	Sign           *ssh.Signature
	Command        Command
	ExecMeta       *ExecMeta
	ForwardDirMeta *ForwardDirMeta
}

type ExecMeta struct {
	Size *pty.Winsize
	Cmd  string
	Args []string
}

type ForwardDirMeta struct {
	Path string
}

type Command int

const (
	CommandExec Command = iota
	CommandForwardSocks5
	CommandForwardDir
)

type Logger interface {
	Info(string, ...slog.Attr)
	Warn(string, ...slog.Attr)
	Error(string, ...slog.Attr)
}

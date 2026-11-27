package domain

import (
	"context"
	"net"
	"net/http"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var ErrHostDocumentNeedsLocalTransport = problem.New(problem.HostDocumentRequiresLocalTransport,
	"a host document is served only over the local socket")

var ErrDaemonStateDocument = problem.New(problem.DaemonStateDocument,
	"the daemon's own state cannot be opened as a document")

type DocumentAuthority string

const (
	DocumentAuthorityWorkspace DocumentAuthority = "workspace"
	DocumentAuthorityHost      DocumentAuthority = "host"
)

type DocumentRef struct {
	Authority   DocumentAuthority `json:"authority"`
	WorkspaceID string            `json:"workspaceId,omitempty"`
	Path        string            `json:"path"`
}

type Document struct {
	DocumentRef
	FileContent
}

type Transport int

const (
	TransportRemote Transport = iota
	TransportLocal
)

func TransportOf(ctx context.Context) Transport {
	addr, ok := ctx.Value(http.LocalAddrContextKey).(net.Addr)
	if ok && addr.Network() == "unix" {
		return TransportLocal
	}
	return TransportRemote
}

package domain

import (
	"context"
	"net"
	"net/http"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ErrHostDocumentNeedsLocalTransport refuses a file outside every workspace to
// a request that did not arrive on the local socket: it is the local user's only.
var ErrHostDocumentNeedsLocalTransport = problem.New(problem.HostDocumentRequiresLocalTransport,
	"a host document is served only over the local socket")

// ErrDaemonStateDocument refuses a document under the daemon's own state
// directory, whose live databases a save would corrupt.
var ErrDaemonStateDocument = problem.New(problem.DaemonStateDocument,
	"the daemon's own state cannot be opened as a document")

// DocumentAuthority names what serves a document: a workspace's worktree, or
// the host filesystem outside every worktree.
type DocumentAuthority string

const (
	DocumentAuthorityWorkspace DocumentAuthority = "workspace"
	DocumentAuthorityHost      DocumentAuthority = "host"
)

// DocumentRef addresses one file. A workspace document is a path relative to
// its worktree, so it survives the worktree moving; a host document is an
// absolute path. An empty Authority is an absolute path not yet resolved.
type DocumentRef struct {
	Authority   DocumentAuthority `json:"authority"`
	WorkspaceID string            `json:"workspaceId,omitempty"`
	Path        string            `json:"path"`
}

// Document is a resolved ref with the file's content.
type Document struct {
	DocumentRef
	FileContent
}

// Transport is how a request reached the daemon. Host documents are served
// only to a request that came over the local socket.
type Transport int

const (
	TransportRemote Transport = iota
	TransportLocal
)

// TransportOf is local only for a request accepted on a unix socket; a request
// that names no listener fails closed as remote.
func TransportOf(ctx context.Context) Transport {
	addr, ok := ctx.Value(http.LocalAddrContextKey).(net.Addr)
	if ok && addr.Network() == "unix" {
		return TransportLocal
	}
	return TransportRemote
}

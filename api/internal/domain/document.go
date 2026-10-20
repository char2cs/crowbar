package domain

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

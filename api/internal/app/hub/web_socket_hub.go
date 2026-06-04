package hub

import "github.com/char2cs/crowbar/api/internal/domain"

// WebSocketHub is the version-agnostic broadcast interface domain producers call.
// Class-B topic methods (Git, Files, LSP, Terminal) are added with their
// broadcasters in later waves.
type WebSocketHub interface {
	BroadcastWorkspace(
		ws domain.Workspace,
	)
	BroadcastChat(
		evt ChatStatusEvent,
	)
}

package hub

import (
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WebSocketHub is the version-agnostic broadcast interface domain producers call.
type WebSocketHub interface {
	BroadcastWorkspace(
		ws domain.Workspace,
	)
	BroadcastGit(
		wsID string,
		status gitdomain.GitStatus,
	)
	BroadcastFile(
		evt domain.FileChangeEvent,
	)
}

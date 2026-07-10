package hub

import (
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WebSocketHub is the version-agnostic broadcast interface domain producers call.
// Each entity broadcast carries its wire DTO (spec §5); the producer resolves any
// derived overlay (e.g. merge eligibility) before calling, so the hub fan-out
// stays a pure dispatch.
type WebSocketHub interface {
	BroadcastProject(
		p dto.ProjectDTO,
	)
	BroadcastRepo(
		r dto.RepoDTO,
	)
	BroadcastWorkspace(
		w dto.WorkspaceDTO,
	)
	BroadcastThread(
		t dto.ThreadDTO,
	)
	BroadcastTerminalSession(
		s dto.TerminalSessionDTO,
	)
	BroadcastGit(
		wsID string,
		status gitdomain.GitStatus,
	)
	BroadcastFile(
		evt domain.FileChangeEvent,
	)
	BroadcastAgentChat(
		chatID string,
		workspaceID string,
		kind string,
	)
}

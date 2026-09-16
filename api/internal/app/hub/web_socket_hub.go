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
	// BroadcastAgentChat carries the chat's folded busy state (working) alongside the
	// kind, so no client re-derives the spinner. See Hub.BroadcastAgentChat.
	BroadcastAgentChat(
		chatID string,
		workspaceID string,
		kind string,
		working bool,
	)
	// BroadcastAgentChatFolder fans a chat-folder lifecycle frame out on the Chats
	// topic. Chat folders are a plain GORM row with no projection to ride, so the
	// mutating handler calls this itself, right after the write. See
	// Hub.BroadcastAgentChatFolder.
	BroadcastAgentChatFolder(
		folderID string,
		workspaceID string,
		kind string,
	)
	// BroadcastAgentRunner carries the CHAT id alongside the runner because the
	// runner's placement IS the event: a `moved` frame names the chat the CLI moved
	// into. See Hub.BroadcastAgentRunner.
	BroadcastAgentRunner(
		runnerID string,
		workspaceID string,
		chatID string,
		kind string,
	)
}

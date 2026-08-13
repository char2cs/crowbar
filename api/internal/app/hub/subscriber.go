package hub

import (
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Subscriber receives hub broadcasts. Implemented by the API WS handler set,
// which fans each entity DTO out to the matching per-entity broadcaster (spec
// §5). The entity topics carry their wire DTOs directly; the Git and File topics
// stay on their domain payloads (the broadcaster serialises them at the edge).
type Subscriber interface {
	PushProject(
		p dto.ProjectDTO,
	)
	PushRepo(
		r dto.RepoDTO,
	)
	PushFolder(
		f dto.FolderDTO,
	)
	PushWorkspace(
		w dto.WorkspaceDTO,
	)
	PushThread(
		t dto.ThreadDTO,
	)
	PushTerminalSession(
		s dto.TerminalSessionDTO,
	)
	PushGit(
		wsID string,
		status gitdomain.GitStatus,
	)
	PushFile(
		evt domain.FileChangeEvent,
	)
	// PushAgentChat receives a chat lifecycle frame. working is the chat's folded
	// busy state as of the event — the server's answer to the spinner, so no client
	// re-derives it from the kind.
	PushAgentChat(
		chatID string,
		workspaceID string,
		kind string,
		working bool,
	)
	// PushAgentChatFolder receives a CHAT FOLDER lifecycle frame
	// (folder_created/folder_updated/folder_deleted). It carries the folder id and
	// nothing else: the Chats socket is a bare event feed with no snapshot, so a
	// frame here means "re-read this workspace's chat folders" — the same thing a
	// reconnect does, which is what makes the live and outage paths repair
	// identically.
	PushAgentChatFolder(
		folderID string,
		workspaceID string,
		kind string,
	)
	// PushAgentRunner receives a runner lifecycle frame
	// (started/session_bound/moved/exited). chatID is the chat the runner is
	// pointed at as of the event — placement, never liveness.
	PushAgentRunner(
		runnerID string,
		workspaceID string,
		chatID string,
		kind string,
	)
}

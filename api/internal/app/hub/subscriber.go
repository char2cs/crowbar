package hub

import (
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
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
	PushWorkspace(
		w dto.WorkspaceDTO,
	)
	// PushAgentChatEvent receives one chat snapshot frame.
	PushAgentChatEvent(ev dto.AgentChatEvent)
	// PushAgentChatTelemetry receives the provider's newest usage report.
	PushAgentChatTelemetry(
		chatID string,
		workspaceID string,
		report agents.Telemetry,
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
}

package v0

import (
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The stream half of each hub push: every row the lifecycle sockets carry
// reaches the projects scope as the entity its snapshot holds.

const statusDeleted = "deleted"

func (c *Container) publishProject(p dto.ProjectDTO) {
	if p.Status == statusDeleted {
		c.app.Stream.Gone(stream.Projects, stream.KindProject, p.ID)
		return
	}
	c.app.Stream.Entity(stream.Projects, stream.KindProject, p.ID, p, 0)
}

func (c *Container) publishRepo(r dto.RepoDTO) {
	if r.Status == statusDeleted {
		c.app.Stream.Gone(stream.Projects, stream.KindRepo, r.ID)
		return
	}
	c.app.Stream.Entity(stream.Projects, stream.KindRepo, r.ID, r, 0)
}

// publishWorkspace sends a tombstoned workspace as gone: no sidebar reader draws one.
func (c *Container) publishWorkspace(w dto.WorkspaceDTO) {
	if w.Status == domain.WorkspaceStatusDeleted {
		c.app.Stream.Gone(stream.Projects, stream.KindWorkspace, w.ID)
		return
	}
	c.app.Stream.Entity(stream.Projects, stream.KindWorkspace, w.ID, w, 0)
}

// publishChat sends the chat's snapshot at its own version; a frame with no
// snapshot is its deletion.
func (c *Container) publishChat(ev dto.AgentChatEvent, scope chatScope) {
	if ev.Chat == nil {
		c.app.Stream.Gone(stream.Projects, stream.KindChatEntity, ev.ChatID)
		return
	}
	c.app.Stream.Entity(stream.Projects, stream.KindChatEntity, ev.ChatID,
		dto.ChatEntityFrom(*ev.Chat, scope.ProjectID, scope.RepoID), ev.Version)
}

// publishTelemetry sends the report on its chat's scope, ordered by when it was observed.
func (c *Container) publishTelemetry(chatID string, report agents.Telemetry) {
	scope := stream.Scope{Kind: stream.KindChat, Key: chatID}
	c.app.Stream.Entity(scope, stream.KindTelemetry, chatID, report, report.ObservedAt.UnixMicro())
}

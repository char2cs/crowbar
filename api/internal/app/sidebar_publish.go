package app

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/stream"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	domainstream "github.com/char2cs/crowbar/api/internal/domain/stream"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The sidebar rows a write changed reach the projects scope as the entities
// its snapshot holds.

const statusDeleted = "deleted"

// PublishProject sends a project a route changed; a deleted one is gone.
func (c *Container) PublishProject(p dto.ProjectDTO) {
	if p.Status == statusDeleted {
		c.Stream.Gone(domainstream.Projects, domainstream.KindProject, p.ID)
		return
	}
	c.Stream.Entity(domainstream.Projects, domainstream.KindProject, p.ID, p, 0)
}

// PublishRepo sends a repo a route changed; a deleted one is gone.
func (c *Container) PublishRepo(r dto.RepoDTO) { publishRepo(c.Stream, r) }

func publishRepo(h *stream.Hub, r dto.RepoDTO) {
	if r.Status == statusDeleted {
		h.Gone(domainstream.Projects, domainstream.KindRepo, r.ID)
		return
	}
	h.Entity(domainstream.Projects, domainstream.KindRepo, r.ID, r, 0)
}

// workspace sends a tombstoned workspace as gone: no sidebar reader draws one.
func (l *lateStream) workspace(w dto.WorkspaceDTO) {
	if w.Status == domain.WorkspaceStatusDeleted {
		l.hub.Gone(domainstream.Projects, domainstream.KindWorkspace, w.ID)
		return
	}
	l.hub.Entity(domainstream.Projects, domainstream.KindWorkspace, w.ID, w, 0)
}

// telemetry sends a usage report on its chat's scope, ordered by when it was observed.
func (l *lateStream) telemetry(chatID, _ string, report agents.Telemetry) {
	scope := domainstream.Scope{Kind: domainstream.KindChat, Key: chatID}
	l.hub.Entity(scope, domainstream.KindTelemetry, chatID, report, report.ObservedAt.UnixMicro())
}

// workspaceGetter reads the workspace a chat's scope comes from.
type workspaceGetter interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

// cwdWalker answers the workspace a bubble's cwd walk lands on.
type cwdWalker interface {
	CwdWorkspaceID(ctx context.Context, chatID string) (string, bool, error)
}

// chatPublisher sends each chat snapshot at its own version, filed under the
// project and repo the repo lists read it from; a frame with no row is its deletion.
// Its reads outlive the boot ctx, which only carries the daemon's values.
func (l *lateStream) chatPublisher(
	ctx context.Context, workspaces workspaceGetter, walk cwdWalker,
) func(agentusecase.ChatSnapshotFrame) {
	ctx = context.WithoutCancel(ctx)
	return func(f agentusecase.ChatSnapshotFrame) {
		chat := f.Snapshot.Chat
		if f.Deleted {
			l.hub.Gone(domainstream.Projects, domainstream.KindChatEntity, chat.ID)
			return
		}
		projectID, repoID := chatScope(ctx, workspaces, walk, chat.ID, chat.WorkspaceID)
		conversation, _ := f.Snapshot.Conversation()
		s := f.Snapshot
		row := dto.AgentChatDTOFrom(chat, dto.ChatSnapshotRuntime(s.Live, s.Phase, s.Version,
			s.TerminalWait, s.AttachedSessionID, conversation, s.Ledger.Busy), nil)
		l.hub.Entity(domainstream.Projects, domainstream.KindChatEntity, chat.ID,
			dto.ChatEntityFrom(row, projectID, repoID), s.Version)
	}
}

// chatScope is the project and repo of the chat's workspace or, for a bubble,
// of the workspace its cwd walk lands on; empty when neither resolves.
func chatScope(ctx context.Context, workspaces workspaceGetter, walk cwdWalker, chatID, workspaceID string) (string, string) {
	if workspaceID == "" {
		id, ok, err := walk.CwdWorkspaceID(ctx, chatID)
		if err != nil || !ok {
			return "", ""
		}
		workspaceID = id
	}
	ws, err := workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", ""
	}
	return ws.ProjectID, ws.RepoID
}

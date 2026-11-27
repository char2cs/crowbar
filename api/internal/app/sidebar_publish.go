package app

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	domainstream "github.com/char2cs/crowbar/api/internal/domain/stream"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (l *lateStream) workspace(w dto.WorkspaceDTO, rev int64) {
	if w.Status == domain.WorkspaceStatusDeleted {
		l.hub.Gone(domainstream.Projects, domainstream.KindWorkspace, w.ID, rev)
		return
	}
	l.hub.Entity(domainstream.Projects, domainstream.KindWorkspace, w.ID, w, rev)
}

func (l *lateStream) telemetry(chatID, _ string, report agents.Telemetry) {
	scope := domainstream.Scope{Kind: domainstream.KindChat, Key: chatID}
	l.hub.Entity(scope, domainstream.KindTelemetry, chatID, report, report.ObservedAt.UnixMicro())
}

func (l *lateStream) telemetryGone(chatID string) {
	scope := domainstream.Scope{Kind: domainstream.KindChat, Key: chatID}
	l.hub.Gone(scope, domainstream.KindTelemetry, chatID, time.Now().UnixMicro())
}

type workspaceGetter interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

type cwdWalker interface {
	CwdWorkspaceID(ctx context.Context, chatID string) (string, bool, error)
}

func (l *lateStream) chatPublisher(
	ctx context.Context, workspaces workspaceGetter, walk cwdWalker,
) func(agentusecase.ChatSnapshotFrame) {
	ctx = context.WithoutCancel(ctx)
	return func(f agentusecase.ChatSnapshotFrame) {
		chat := f.Snapshot.Chat
		if f.Deleted {
			l.hub.Gone(domainstream.Projects, domainstream.KindChatEntity, chat.ID, f.Snapshot.Version)
			return
		}
		projectID, repoID := chatScope(ctx, workspaces, walk, chat.ID, chat.WorkspaceID)
		conversation, _ := f.Snapshot.Conversation()
		s := f.Snapshot
		rt := dto.ChatSnapshotRuntime(s.Live, s.Phase, s.Version,
			s.TerminalWait, s.AttachedSessionID, conversation, s.Ledger.Busy)
		rt.PendingMove = s.Ledger.PendingMove
		row := dto.AgentChatDTOFrom(chat, rt, nil)
		l.hub.Entity(domainstream.Projects, domainstream.KindChatEntity, chat.ID,
			dto.ChatEntityFrom(row, projectID, repoID), s.Version)
	}
}

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

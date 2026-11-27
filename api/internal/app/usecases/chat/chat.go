package chat

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/conversation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/defaultlevel"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/manifestfetch"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type ChatUsecase interface {
	ChatSnapshot(ctx context.Context, chatID string) (ChatSnapshot, error)

	ChatSnapshotsOf(ctx context.Context, chatIDs []string) ([]ChatSnapshot, error)

	MintChat(
		ctx context.Context,
		chatID string,
		workspaceID string,
		providerID string,
		surface string,
	) (string, error)

	RenameChat(
		ctx context.Context,
		chatID, title, source string,
	) error

	RenameByRunner(
		ctx context.Context,
		runnerID, title, source string,
	) error

	PurgeChat(
		ctx context.Context,
		chatID string,
	) error

	OpenFeed(ctx context.Context, chatID string, since, windowBytes int64) (<-chan FeedFrame, error)
	ReadRows(ctx context.Context, chatID string, before, bytes int64) (RowsPage, error)
	ReadRow(ctx context.Context, chatID string, seq int64) (ledger.Row, error)
	ReadBlob(ctx context.Context, chatID, ref string) ([]byte, error)

	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)

	BackfillChatTypes(
		ctx context.Context,
	) (skipped int, err error)

	UntypedChats(
		ctx context.Context,
	) ([]string, error)

	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)

	ListChatsInRepo(
		ctx context.Context,
		repoID string,
	) ([]domain.Chat, error)

	CwdWorkspaceID(
		ctx context.Context,
		chatID string,
	) (string, bool, error)

	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)

	SetChatSelection(
		ctx context.Context,
		chatID string,
		model string,
		effort string,
	) error

	ReadChatLog(
		ctx context.Context,
		chatID string,
	) ([]agenttools.ChatTurn, error)

	NoteThreadLineage(
		ctx context.Context,
		chatID string,
		ancestors []string,
	) error

	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)

	AssembleHandoff(
		ctx context.Context,
		chatID string,
	) (string, error)
}

var _ ChatUsecase = (*Usecase)(nil)

type Usecase struct {
	chats     agentchat.EventStore
	agents    engineagents.Agents
	ledger    *ledgerstore.Store
	blobs     *ledgerstore.Blobs
	ws        WorkspaceReader
	home      func() (string, error)
	worktree  WorktreeCreator
	folders   TreeFolders
	nodes     TreeNodes
	repoRoots TreeRepoRoots
	answers   *answerdesk.Desk
	asks      askResolver
	tools     agenttools.Deps

	snapshots *snapshot.Snapshots
	feed      atomic.Pointer[ChatFeed]
	feeds     *feeds

	conversations *conversation.Conversations
	recorder      *ingress.Recorder
	runners       *runner.Runners
	ingress       *ingress.Ingress
	providers     *provider.Providers
	defaultLevel  *defaultlevel.DefaultLevel
	manifestFetch *manifestfetch.ManifestFetch
}

func (u *Usecase) MintChat(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	surface string,
) (string, error) {
	return u.conversations.MintChat(ctx, chatID, workspaceID, providerID, surface)
}

func (u *Usecase) RenameChat(
	ctx context.Context,
	chatID, title, source string,
) error {
	return u.conversations.RenameChat(ctx, chatID, title, source)
}

func (u *Usecase) RenameByRunner(
	ctx context.Context,
	runnerID, title, source string,
) error {
	return u.conversations.RenameByRunner(ctx, runnerID, title, source)
}

func (u *Usecase) PurgeChat(
	ctx context.Context,
	chatID string,
) error {
	err := u.conversations.PurgeChat(ctx, chatID)
	if errors.Is(err, agentchat.ErrNotFound) {
		return fmt.Errorf("%w: %w", err, problem.ErrNotFound)
	}
	if err == nil {
		u.feeds.deleted(chatID)
	}
	return err
}

func (u *Usecase) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return u.conversations.ListChats(ctx)
}

func (u *Usecase) BackfillChatTypes(
	ctx context.Context,
) (int, error) {
	return u.conversations.BackfillChatTypes(ctx)
}

func (u *Usecase) UntypedChats(
	ctx context.Context,
) ([]string, error) {
	return u.conversations.UntypedChats(ctx)
}

func (u *Usecase) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.Chat, error) {
	return u.conversations.ListChatsByWorkspace(ctx, workspaceID)
}

func (u *Usecase) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return u.conversations.GetChat(ctx, id)
}

func (u *Usecase) SetChatSelection(
	ctx context.Context,
	chatID, model, effort string,
) error {
	return u.runners.SetChatSelection(ctx, chatID, model, effort)
}

func (u *Usecase) ReadChatLog(
	ctx context.Context,
	chatID string,
) ([]agenttools.ChatTurn, error) {
	return u.conversations.ReadChatLog(ctx, chatID)
}

func (u *Usecase) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	return u.conversations.NoteThreadLineage(ctx, chatID, ancestors)
}

func (u *Usecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.conversations.AssembleHandoff(ctx, chatID)
}

func (u *Usecase) Ancestors(
	ctx context.Context,
	chatID string,
) ([]string, error) {
	return u.conversations.Ancestors(ctx, chatID)
}

func (u *Usecase) ChatSnapshot(ctx context.Context, chatID string) (ChatSnapshot, error) {
	return u.snapshots.Get(ctx, chatID)
}

func (u *Usecase) ChatSnapshotsOf(ctx context.Context, chatIDs []string) ([]ChatSnapshot, error) {
	return u.snapshots.GetMany(ctx, chatIDs)
}

func (u *Usecase) Snapshots() *ChatSnapshots { return u.snapshots }

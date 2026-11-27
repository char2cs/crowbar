package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/fanout"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
)

type (
	ToolDeps        = tools.Deps
	ToolSet         = tools.ToolSet
	TokenMinter     = tools.TokenMinter
	ToolIdempotency = tools.Idempotency
	ToolMetrics     = tools.Metrics
	ToolStat        = tools.ToolStat
	ChatTurn        = tools.ChatTurn

	ToolChatReader             = tools.ChatReader
	ToolChatLogReader          = tools.ChatLogReader
	ToolChatLineageReader      = tools.ChatLineageReader
	ToolReviewReader           = tools.ReviewReader
	ToolThreadBroadcast        = tools.ThreadBroadcast
	ToolWorkspaceBranchRenamer = tools.WorkspaceBranchRenamer
	ToolBrowserBridge          = tools.BrowserBridge
)

type (
	TreeUsecase            = tree.Usecase
	TreeChats              = tree.Chats
	TreeAgent              = tree.Agent
	TreeWorkspaceGitStatus = tree.WorkspaceGitStatus
	TreeWorkspaceReaper    = tree.WorkspaceReaper
	TreeWorkspaceHolders   = tree.WorkspaceHolders
	TreeFolders            = tree.Folders
	TreeNodes              = tree.Nodes
	TreeRepoRoots          = tree.RepoRoots

	CreateInput  = tree.CreateInput
	MoveInput    = tree.MoveInput
	PlaceInput   = tree.PlaceInput
	ChatDeletion = tree.ChatDeletion

	WorktreeSpec    = tree.WorktreeSpec
	WorktreeMode    = tree.WorktreeMode
	ImportSpec      = tree.ImportSpec
	CreateChatInput = tree.CreateChatInput
	CreatedChat     = tree.CreatedChat
)

const (
	WorktreeNone   = tree.WorktreeNone
	WorktreeFork   = tree.WorktreeFork
	WorktreeImport = tree.WorktreeImport
)

func NewTokenMinter() (*TokenMinter, error) { return tools.NewTokenMinter() }

func NewToolSet(deps ToolDeps, runnerID, token string) *ToolSet {
	return tools.NewToolSet(deps, runnerID, token)
}

func NewToolIdempotency() *ToolIdempotency { return tools.NewIdempotency() }

func NewToolMetrics() *ToolMetrics { return tools.NewMetrics() }

func NewTree(
	chats TreeChats,
	agent TreeAgent,
	busy TreeBusy,
	workspaces TreeWorkspaceGitStatus,
	reaper TreeWorkspaceReaper,
	holders TreeWorkspaceHolders,
	folders TreeFolders,
	nodes TreeNodes,
) TreeUsecase {
	return tree.New(chats, agent, busy, workspaces, reaper, holders, folders, nodes)
}

type TreeBusy = tree.Busy

func (u *Usecase) ChatBusy(ctx context.Context, chatID string) (bool, error) {
	return u.runners.ChatBusy(ctx, chatID)
}

func NewChatLineage(chats TreeChats) ChatLineage {
	return tree.NewLineage(chats)
}

type (
	TerminalCommander = seam.TerminalCommander
	WorkspaceReader   = seam.WorkspaceReader
	ChatLineage       = seam.ChatLineage
	ChatFeed          = seam.ChatFeed
)

var (
	ErrToolUnauthorized           = tools.ErrUnauthorized
	ErrChatProviderUnknown        = runner.ErrChatProviderUnknown
	ErrNoLiveRunner               = runner.ErrNoLiveRunner
	ErrStopped                    = runner.ErrStopped
	ErrTreeNameRequired           = tree.ErrNameRequired
	ErrNoRepoToBranch             = tree.ErrNoRepoToBranch
	ErrTreeCycle                  = tree.ErrCycle
	ErrTreeCrossWorkspace         = tree.ErrCrossWorkspace
	ErrTreeCrossContext           = tree.ErrCrossContext
	ErrTreeForkChain              = tree.ErrForkChain
	ErrTreeSubtreeWorking         = tree.ErrSubtreeWorking
	ErrTreeWorkspaceUnprovisioned = tree.ErrWorkspaceUnprovisioned
)

type Fanout = fanout.Fanout

func NewFanout(snaps *ChatSnapshots) *Fanout { return fanout.New(snaps) }

type ChatSnapshots = snapshot.Snapshots

type ChatSnapshot = snapshot.Snapshot

type ChatSnapshotFrame = snapshot.Frame

func NewChatSnapshots() *ChatSnapshots { return snapshot.NewAtBoot() }

const (
	ChatPhaseDormant   = snapshot.PhaseDormant
	ChatPhaseStarting  = snapshot.PhaseStarting
	ChatPhaseLive      = snapshot.PhaseLive
	ChatPhaseSwitching = snapshot.PhaseSwitching
	ChatPhaseStopping  = snapshot.PhaseStopping
)

var _ = agentchat.WatchFunc(nil)

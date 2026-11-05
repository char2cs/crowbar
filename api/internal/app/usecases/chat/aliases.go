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

// The door for everything the composition root builds itself (tool surface, tree
// usecase, fanout, their seams and sentinels) — none of it is a method on *Usecase.

// The agent-facing MCP tool surface, re-exported: the feature exposes it rather than
// another layer composing it. Prefixed Tool so one import carries this and the tree.
type (
	// ToolDeps is everything the tool surface needs wired before it can serve a
	// call. Its Chats and ChatLogs ports are filled in by New; the rest come from
	// the composition root.
	ToolDeps = tools.Deps
	// ToolSet is one authenticated caller's view of the surface.
	ToolSet = tools.ToolSet
	// TokenMinter issues and verifies the per-runner token an MCP call is
	// authenticated by. There is exactly ONE per daemon: a runner's token must be
	// minted by the same secret that verifies it.
	TokenMinter = tools.TokenMinter
	// ToolIdempotency deduplicates repeated writes from one agent.
	ToolIdempotency = tools.Idempotency
	// ToolMetrics counts calls per tool. It is the one fail-OPEN dependency: a
	// missing counter must never refuse a call.
	ToolMetrics = tools.Metrics
	// ToolStat is one tool's call count, as the status endpoint reports it.
	ToolStat = tools.ToolStat
	// ChatTurn is one turn of a chat as the tool surface renders it.
	ChatTurn = tools.ChatTurn

	// ToolChatReader, ToolChatLogReader and ToolChatLineageReader are the read
	// seams the surface answers chat questions through.
	ToolChatReader        = tools.ChatReader
	ToolChatLogReader     = tools.ChatLogReader
	ToolChatLineageReader = tools.ChatLineageReader
	// ToolReviewReader is the code-review seam.
	ToolReviewReader = tools.ReviewReader
	// ToolThreadBroadcast announces a review thread an agent just wrote.
	ToolThreadBroadcast = tools.ThreadBroadcast
	// ToolWorkspaceBranchRenamer is set_branch_name's write seam onto the workspace
	// usecase. A nil one silently withdraws the tool, so the root must name it to
	// refuse to start without it.
	ToolWorkspaceBranchRenamer = tools.WorkspaceBranchRenamer
	// ToolBrowserBridge is the browser tools' route to the desktop app.
	ToolBrowserBridge = tools.BrowserBridge
)

// The sidebar forest's tree, re-exported: a separate usecase with its own routes, but
// part of this feature — it moves chat rows and its delete cascades into the chat aggregate.
type (
	// TreeUsecase is the folder/placement surface the /folders routes are served
	// off.
	TreeUsecase = tree.Usecase
	// TreeChats is the chat-aggregate surface the tree reads and re-places;
	// folder rows and conversation rows share one table.
	TreeChats = tree.Chats
	// TreeAgent is what the tree asks to erase each chat a cascade decided must go.
	TreeAgent = tree.Agent
	// TreeWorkspaceGitStatus is the tree's read seam onto the workspace layer.
	TreeWorkspaceGitStatus = tree.WorkspaceGitStatus
	// TreeWorkspaceReaper is the teardown a cascading chat delete puts each
	// worktree in its subtree through, so a workspace never outlives the chat
	// that owned it.
	TreeWorkspaceReaper = tree.WorkspaceReaper
	// TreeWorkspaceHolders is every chat currently resolving to a workspace, so a
	// cascade never tears down a worktree surviving siblings still hold.
	TreeWorkspaceHolders = tree.WorkspaceHolders
	// TreeFolders is the plain-GORM identity surface a home-scoped folder's
	// name lives on.
	TreeFolders = tree.Folders
	// TreeNodes is the position surface a home-scoped chat or folder's
	// placement goes through instead of Chat.SetOrder/.SetPlacement.
	TreeNodes = tree.Nodes
	// TreeRepoRoots answers a repo's default checkout for a walk that reaches
	// the panel root through a repo-scoped folder — see tree.RepoRoots.
	TreeRepoRoots = tree.RepoRoots

	// CreateInput, MoveInput and PlaceInput are the three writes the panel makes.
	CreateInput = tree.CreateInput
	MoveInput   = tree.MoveInput
	PlaceInput  = tree.PlaceInput
	// ChatDeletion is what a cascading chat delete removed.
	ChatDeletion = tree.ChatDeletion

	// WorktreeSpec, WorktreeMode and ImportSpec are CreateChat's worktree half: no
	// worktree, a fresh fork, or an existing branch (and which). Re-exported because
	// the composition root constructs one to wire the import paths.
	WorktreeSpec = tree.WorktreeSpec
	WorktreeMode = tree.WorktreeMode
	ImportSpec   = tree.ImportSpec
	// CreateChatInput and CreatedChat are CreateChat's request and answer.
	CreateChatInput = tree.CreateChatInput
	CreatedChat     = tree.CreatedChat
)

// The three worktree modes a create can ask for, re-exported alongside the
// spec they live on.
const (
	// WorktreeNone is a plain chat — a bubble or a thread — owning no worktree.
	WorktreeNone = tree.WorktreeNone
	// WorktreeFork mints a fresh branch off the chat's resolved fork parent.
	WorktreeFork = tree.WorktreeFork
	// WorktreeImport adopts a branch that already exists in the repository.
	WorktreeImport = tree.WorktreeImport
)

// NewTokenMinter mints the daemon's single MCP token secret.
func NewTokenMinter() (*TokenMinter, error) { return tools.NewTokenMinter() }

// NewToolSet builds one authenticated caller's tool surface.
func NewToolSet(deps ToolDeps, runnerID, token string) *ToolSet {
	return tools.NewToolSet(deps, runnerID, token)
}

// NewToolIdempotency returns an empty write-deduplication ledger.
func NewToolIdempotency() *ToolIdempotency { return tools.NewIdempotency() }

// NewToolMetrics returns an empty per-tool call counter.
func NewToolMetrics() *ToolMetrics { return tools.NewMetrics() }

// NewTree builds the sidebar forest's tree usecase. busy (Usecase.ChatBusy) lets
// its move and delete verbs refuse over a subtree that is still working.
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

// TreeBusy is the tree usecase's question of whether a chat is working.
type TreeBusy = tree.Busy

// ChatBusy reports whether chatID is working, as its ledger folds it.
func (u *Usecase) ChatBusy(ctx context.Context, chatID string) (bool, error) {
	return u.runners.ChatBusy(ctx, chatID)
}

// NewChatLineage builds the lineage reader over the chat repository. It is
// built BEFORE the chat usecase and handed to it, because the tree usecase
// that owns the same edges holds the chat usecase in turn.
func NewChatLineage(chats TreeChats) ChatLineage {
	return tree.NewLineage(chats)
}

// The seams this feature reaches the rest of the daemon through, re-exported so
// the composition root can name what it must satisfy.
type (
	// TerminalCommander is the PTY seam every vendor CLI is started, ended and
	// inspected through.
	TerminalCommander = seam.TerminalCommander
	// WorkspaceReader resolves the on-disk locations one workspace's agent work
	// happens in.
	WorkspaceReader = seam.WorkspaceReader
	// ChatLineage answers "what does this chat read" at spawn time.
	ChatLineage = seam.ChatLineage
	// ChatFeed is where the usecase publishes the live facts about a chat that
	// no aggregate projection carries; the hub's owner binds it at sweep start.
	ChatFeed = seam.ChatFeed
)

// Sentinels of the internal packages this feature's door re-exports, so a caller
// can classify their failures without importing past the door.
var (
	// ErrToolUnauthorized is a runner callback that could not prove it is the
	// runner it claims to be.
	ErrToolUnauthorized = tools.ErrUnauthorized
	// ErrChatProviderUnknown is a resume of a dormant chat that records no
	// provider. It is DISTINCT from a missing runner, which is the ordinary
	// dormant state and the reason the resume was asked for in the first place.
	ErrChatProviderUnknown = runner.ErrChatProviderUnknown
	// ErrNoLiveRunner is a chat with no CLI on it: dormant, the normal state
	// between runs.
	ErrNoLiveRunner = runner.ErrNoLiveRunner
	// ErrStopped is a switch or resume that was parked when the user pressed
	// Stop; Stop preempted it and it changed nothing.
	ErrStopped = runner.ErrStopped
	// ErrTreeNameRequired is a folder create or rename with a blank name.
	ErrTreeNameRequired = tree.ErrNameRequired
	// ErrTreeCycle is a move that would make a node its own ancestor.
	ErrTreeCycle = tree.ErrCycle
	// ErrTreeCrossWorkspace is a move whose destination belongs to another
	// workspace.
	ErrTreeCrossWorkspace = tree.ErrCrossWorkspace
	// ErrTreeCrossContext is a folder MOVE that would cross from one context
	// (project home, a bare repo root, or one specific branch's own
	// workspace) to a different one, even within the same repo.
	ErrTreeCrossContext = tree.ErrCrossContext
	// ErrTreeForkChain is a workspace placement away from its fork parent.
	ErrTreeForkChain = tree.ErrForkChain
	// ErrTreeSubtreeWorking is a move or delete refused because the row or a
	// row in the subtree it takes is currently working.
	ErrTreeSubtreeWorking = tree.ErrSubtreeWorking
	// ErrTreeWorkspaceUnprovisioned is a create into a workspace that has no
	// worktree on disk — a placeholder, which the chat would have nowhere to
	// run in.
	ErrTreeWorkspaceUnprovisioned = tree.ErrWorkspaceUnprovisioned
)

// Fanout feeds repository lifecycle announcements to the chat snapshot owner. The
// composition root builds it, not chat.New: the repositories need their watch seams
// at construction, before the usecase exists.
type Fanout = fanout.Fanout

// NewFanout builds the fanout over the snapshot owner. A nil owner degrades to
// a no-op.
func NewFanout(snaps *ChatSnapshots) *Fanout { return fanout.New(snaps) }

// ChatSnapshots is the ONE owner of every chat's versioned snapshot. The composition
// root builds it (the fanout needs it before the usecase exists) and hands it to New.
type ChatSnapshots = snapshot.Snapshots

// ChatSnapshot is one versioned answer about a chat.
type ChatSnapshot = snapshot.Snapshot

// ChatSnapshotFrame is one published change to a chat's snapshot.
type ChatSnapshotFrame = snapshot.Frame

// NewChatSnapshots returns an owner whose versions start at the boot time.
func NewChatSnapshots() *ChatSnapshots { return snapshot.NewAtBoot() }

// The chat phases a snapshot reports.
const (
	ChatPhaseDormant   = snapshot.PhaseDormant
	ChatPhaseStarting  = snapshot.PhaseStarting
	ChatPhaseLive      = snapshot.PhaseLive
	ChatPhaseSwitching = snapshot.PhaseSwitching
	ChatPhaseStopping  = snapshot.PhaseStopping
)

var _ = agentchat.WatchFunc(nil)

package chat

import (
	"context"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/fanout"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// This file is the door for everything the COMPOSITION ROOT builds itself: the
// tool surface, the tree usecase, the fanout, the seams they need and the
// sentinels they return. None of it is a method on *Usecase, which is why none of
// it lives in one of the five responsibility files.

// The agent-facing capability surface, re-exported.
//
// tools is an internal package: the MCP tool set is not a thing another layer
// composes, it is a thing this feature exposes. What the composition root and the
// status mapper legitimately name is re-exported here, prefixed Tool so one
// import can carry both this and the tree without collisions.
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
	// ToolResolver authenticates a caller and computes what it may see.
	ToolResolver = tools.Resolver
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
	// ToolRunnerReader, ToolChatGetter and ToolWorkspaceLister are what a resolver
	// authenticates and scopes a caller against.
	ToolRunnerReader    = tools.RunnerReader
	ToolChatGetter      = tools.ChatGetter
	ToolWorkspaceLister = tools.WorkspaceLister
	// ToolWorkspaceBranchRenamer is set_branch_name's write seam onto the
	// workspace usecase. It is re-exported for the composition root's sake: the
	// tool is withdrawn silently when it is nil, so the root has to be able to
	// name the port it must refuse to start without.
	ToolWorkspaceBranchRenamer = tools.WorkspaceBranchRenamer
)

// The sidebar forest's tree, re-exported.
//
// The tree is a separate usecase with its own routes, but it is part of THIS
// feature: it moves chat rows, and its chat delete cascades into the chat
// aggregate. It is reached through this door so the feature has one.
type (
	// TreeUsecase is the folder/placement surface the /folders routes are served
	// off.
	TreeUsecase = tree.Usecase
	// TreeChats is the chat-aggregate surface the tree reads and re-places —
	// folder rows and conversation rows are the same table now.
	TreeChats = tree.Chats
	// TreeAgent is what the tree asks to erase each chat a cascade decided must go.
	TreeAgent = tree.Agent
	// TreeWorkspaceGitStatus is DeletePreview's read onto each workspace-owning
	// row's already-synced uncommitted file counts.
	TreeWorkspaceGitStatus = tree.WorkspaceGitStatus
	// TreeWorkspaceRoster is the boot backfill's census of every workspace the
	// daemon knows.
	TreeWorkspaceRoster = tree.WorkspaceRoster

	// CreateInput, MoveInput and PlaceInput are the three writes the panel makes.
	CreateInput = tree.CreateInput
	MoveInput   = tree.MoveInput
	PlaceInput  = tree.PlaceInput
	// ChatDeletion is what a cascading chat delete removed.
	ChatDeletion = tree.ChatDeletion
)

// NewTokenMinter mints the daemon's single MCP token secret.
func NewTokenMinter() (*TokenMinter, error) { return tools.NewTokenMinter() }

// NewToolSet builds one authenticated caller's tool surface.
func NewToolSet(deps ToolDeps, runnerID, token string) *ToolSet {
	return tools.NewToolSet(deps, runnerID, token)
}

// NewToolResolver builds the authenticator and scoper for MCP callers.
func NewToolResolver(
	minter *TokenMinter,
	runners ToolRunnerReader,
	chats ToolChatGetter,
	workspaces ToolWorkspaceLister,
) *ToolResolver {
	return tools.NewResolver(minter, runners, chats, workspaces)
}

// NewToolIdempotency returns an empty write-deduplication ledger.
func NewToolIdempotency() *ToolIdempotency { return tools.NewIdempotency() }

// NewToolMetrics returns an empty per-tool call counter.
func NewToolMetrics() *ToolMetrics { return tools.NewMetrics() }

// ResolveOwningChat picks the chat that owns a workspace from its candidate
// rows — typically ListChatsByWorkspace's result — applying the tree
// package's own branch-preference tiebreak (see tree.ResolveOwningChat).
// Re-exported through this door so a caller with no other business in this
// feature (the workspace wire DTO, wiring owningChatId) can answer "which
// chat owns this workspace" without re-deriving that rule a second time.
func ResolveOwningChat(rows []domain.Chat) (domain.Chat, bool) {
	return tree.ResolveOwningChat(rows)
}

// NewTree builds the sidebar forest's tree usecase. work is the chat
// usecase's own in-flight tracker (see Usecase.Work) — the tree's move and
// delete verbs refuse over a subtree that is still working, and there is
// exactly one tracker to ask. workspaces is DeletePreview's seam onto the
// workspace layer; roster is the boot backfill's.
func NewTree(
	chats TreeChats,
	agent TreeAgent,
	work *inflight.Work,
	workspaces TreeWorkspaceGitStatus,
	roster TreeWorkspaceRoster,
) TreeUsecase {
	return tree.New(chats, agent, work, workspaces, roster)
}

// Work exposes the in-flight turn tracker this usecase's own components
// observe, so the tree usecase built on top of it (see NewTree) can refuse a
// move or delete over a chat that is currently working. Deliberately not on
// ChatUsecase: only the composition root wiring the tree usecase needs it.
func (u *Usecase) Work() *inflight.Work {
	return u.work
}

// HasTurns reports whether anything was ever SAID in a chat.
//
// Here rather than in one of the five responsibility files for the same reason
// as Work and Working above: the only caller is the tree usecase built on top
// of this one, whose boot backfill asks it before adopting a row into a
// different kind (see tree.Agent.HasTurns). It is the same "has this chat said
// anything yet" test NoteThreadLineage already makes, kept to one read of the
// turn record rather than rendering a log nobody reads.
func (u *Usecase) HasTurns(
	ctx context.Context,
	chatID string,
) (bool, error) {
	turns, err := u.conversations.ChatTurns(ctx, chatID)
	if err != nil {
		return false, err
	}
	return len(turns) > 0, nil
}

// Working reports whether chatID is currently working — the SAME live,
// process-local answer Work() exposes the tree usecase's guardNotWorking, in
// the narrow bool shape usecases/workspace's own reparent guard needs. It is
// exposed here, rather than through inflight.Work directly, so that package
// (this feature's own turn/runner machinery) never has to be imported by a
// consumer with no other business depending on it (usecases/workspace may not
// import usecases/chat/internal/shared/inflight, an internal package).
func (u *Usecase) Working(chatID string) bool {
	working, _, _ := u.work.Observe(chatID)
	return working
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
	// Stall is a turn the SCREEN says will never finish — a usage limit, a service
	// outage. It crosses from the detector that recognises it to the ingress that
	// closes the turn, which is why it is vocabulary rather than either one's.
	Stall = seam.Stall
)

// Sentinels of the internal packages this feature's door re-exports, so a caller
// can classify their failures without importing past the door.
var (
	// ErrToolUnauthorized is a runner callback that could not prove it is the
	// runner it claims to be.
	ErrToolUnauthorized = tools.ErrUnauthorized
	// ErrTreeNameRequired is a folder create or rename with a blank name.
	ErrTreeNameRequired = tree.ErrNameRequired
	// ErrTreeCycle is a move that would make a node its own ancestor.
	ErrTreeCycle = tree.ErrCycle
	// ErrTreeCrossWorkspace is a move whose destination belongs to another
	// workspace.
	ErrTreeCrossWorkspace = tree.ErrCrossWorkspace
	// ErrTreeSubtreeWorking is a move or delete refused because the row or a
	// row in the subtree it takes is currently working.
	ErrTreeSubtreeWorking = tree.ErrSubtreeWorking
)

// Fanout shapes repository lifecycle announcements into frontend frames.
//
// It is built by the composition root rather than by chat.New because the chat
// repositories are constructed BEFORE the usecase that reads them, and they need
// their watch seams at construction. The decision of what a client is told still
// lives here, in the usecase layer, which is the whole point of the seam.
type Fanout = fanout.Fanout

// Hub is the WS broadcaster the fanout needs. *hub.Hub satisfies it.
type Hub = fanout.Hub

// NewFanout builds the fanout over hub. A nil hub degrades to a no-op.
func NewFanout(hub Hub) *Fanout { return fanout.New(hub) }

var (
	_ = agentchat.WatchFunc(nil)
	_ = agentrunner.WatchFunc(nil)
)

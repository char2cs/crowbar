package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// WorktreeResolver resolves a chat to the workspace behind its worktree.
type WorktreeResolver interface {
	Resolve(ctx context.Context, chatID string) (domain.Workspace, error)
}

// worktreeResolver adapts the worktree package's functions over the
// container's own chat, workspace, folder and node reads (law 4).
type worktreeResolver struct {
	chats      worktree.ChatAncestryReader
	chatRows   worktree.ChatLister
	workspaces worktree.WorkspaceReader
	folders    worktree.Folders
	nodes      worktree.Nodes
}

// Resolve implements WorktreeResolver.
func (r worktreeResolver) Resolve(
	ctx context.Context,
	chatID string,
) (domain.Workspace, error) {
	return worktree.Resolve(ctx, chatID, r.chats, r.workspaces)
}

// ChatsForWorkspace answers the chat tree's delete census (WorkspaceHolders).
func (r worktreeResolver) ChatsForWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]string, error) {
	return worktree.ChatsForWorkspace(ctx, workspaceID, r.chatRows, r.folders, r.nodes)
}

// agentWiring is the agentic surface as one value: the chat usecase, the Chats
// panel's tree usecase built on top of it, and the two handles the container has
// to keep hold of beside them — the workspace reader the app layer reuses for the
// delete cascade's on-disk reap, and the tool metrics nothing else can reach.
type agentWiring struct {
	chat     *agentusecase.Usecase
	chatTree agentusecase.TreeUsecase
	worktree worktreeResolver
	wsReader agentusecase.WorkspaceReader
	metrics  *agentusecase.ToolMetrics
}

func newAgentWiring(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
	crowbarHome func() (string, error),
	review agentusecase.ToolReviewReader,
	threadBroadcast agentusecase.ToolThreadBroadcast,
	workspaceUsecase workspace.Usecase,
	chatSnapshots *agentusecase.ChatSnapshots,
	browser agentusecase.ToolBrowserBridge,
) (agentWiring, error) {
	wsReader := &agentWorkspaceReader{
		workspaces:  repos.Workspace,
		repos:       gormStores.Repositories,
		crowbarHome: crowbarHome,
	}
	minter, err := agentusecase.NewTokenMinter()
	if err != nil {
		return agentWiring{}, fmt.Errorf("usecases: new container: %w", err)
	}
	// The Chats-panel lineage read, built first from the chat repository: taking it off
	// the tree usecase would close a construction cycle. Wrapped, since a chat's ParentID
	// is frozen at creation and this read decides what a spawned CLI is told to read.
	lineage := agentusecase.NewChatLineage(agentusecase.NewHomeCorrectedTreeChats(
		repos.AgentChat, repos.Placement, repos.Placement,
	))
	toolDeps, err := newAgentToolDeps(minter, repos, review, threadBroadcast, workspaceUsecase, browser)
	if err != nil {
		return agentWiring{}, err
	}
	chat := agentusecase.New(agentusecase.Deps{
		Chats:                   repos.AgentChat,
		Ledger:                  repos.ChatLedger,
		Blobs:                   repos.ChatBlobs,
		Agents:                  engines.Agents,
		Terminal:                engines.Terminal,
		Workspace:               wsReader,
		Worktree:                worktreeChildCreator{worktree: workspaceUsecase},
		Lineage:                 lineage,
		ProviderPrefs:           gormStores.AgentProviderPreferences,
		PermissionPrefs:         gormStores.AgentPermissionDefault,
		ModelManifestFetchPrefs: gormStores.AgentModelManifestFetch,
		ChatTelemetry:           gormStores.AgentChatTelemetry,
		Home:                    crowbarHome,
		DaemonAddress:           engines.DaemonAddress,
		CrowbarBin:              engines.CrowbarBin,
		NoAPITransport:          engines.NoAPITransport,
		// Installed is left nil: the usecase defaults to Agent.Installed, the real
		// install probe. Only tests inject a stub to isolate from the host PATH.
		Minter:    minter,
		Tools:     toolDeps,
		Snapshots: chatSnapshots,
		// Folders/Nodes let the ancestor walks see past a Folder-only ancestor.
		Folders:      repos.Placement,
		Nodes:        repos.Placement,
		RepoRoots:    workspaceGitStatusReader{workspace: workspaceUsecase, repos: gormStores.Repositories},
		AgentWorking: repos.SetAgentWorking,
		AnswerClock:  time.After,
	})
	// The chat→worktree resolver, built here because it reads the chat forest
	// off the usecase above and because the tree below needs its inverse; the
	// chat-scoped routes resolve through this same value.
	worktreeUsecase := worktreeResolver{
		chats:      worktree.NewChatTreeAncestryReader(chat, repos.Placement, repos.Placement),
		chatRows:   chat,
		workspaces: workspaceUsecase,
		folders:    repos.Placement,
		nodes:      repos.Placement,
	}
	chatTree := agentusecase.NewTree(
		repos.AgentChat,
		chat,
		chat,
		workspaceGitStatusReader{workspace: workspaceUsecase, repos: gormStores.Repositories},
		// The SAME adapter the chat usecase's own WorktreeCreator is satisfied
		// with, handed here a second time on purpose: a cascading delete and a
		// failed promotion's rollback must tear a workspace down through one call
		// (hierarchy.DeleteCascade), not two implementations that could drift.
		worktreeChildCreator{worktree: workspaceUsecase},
		// And the census that gates it: Chat.WorkspaceID names the worktree a
		// chat is anchored to, never that it is anchored there alone, so the
		// delete asks who else is currently resolving to it before cascading.
		worktreeUsecase,
		// Home-scoped folders and chat placement go through Folder/Node.
		repos.Placement,
		repos.Placement,
	)
	return agentWiring{
		chat:     chat,
		chatTree: chatTree,
		worktree: worktreeUsecase,
		wsReader: wsReader,
		metrics:  toolDeps.Metrics,
	}, nil
}

// newAgentToolDeps assembles the production agent capability surface and REFUSES
// to return a partial one.
//
// agenttools registers no tool whose dependency is missing, which is the right
// default for the package but a terrible one to discover in production: a daemon
// wired with a nil port would boot clean, serve chats normally, and hand every
// agent an empty tool list with nothing anywhere reporting why. Checking here
// turns that silent degradation into a failed start.
// The review ports need no adapter: branchreview.Usecase already has
// GetScope/GetOutline with agentusecase.ToolReviewReader's exact signatures, and
// reviewthread.ReviewThread already satisfies BOTH the read and the write half of
// the thread port, so the same repository is handed to Threads and ThreadWrites.
// The Idempotency map is built HERE, once, because it must outlive the per-request
// ToolSet for a retried post_review_comment to be recognized as a retry.
//
// threadBroadcast is injected from the app layer rather than derived here: fanning
// a thread out needs the wire DTO, and a usecase must not import the api layer's
// wire types. See agentusecase.ToolThreadBroadcast.
//
// workspaces is the workspace usecase itself, which already satisfies
// set_branch_name's narrow WorkspaceBranchRenamer port by name. It is a
// parameter rather than something read off repos because renaming a branch is a
// USECASE verb — a git ref rename plus one record write, with the locked/held
// refusals in front of it — not a store write. It is in the refusal switch for
// the reason every other port is: a nil here withdraws set_branch_name, and an
// agent that cannot name its own branch leaves every generated provisional name
// in place with nothing anywhere reporting why.
//
// ChatLogs is deliberately NOT set here, unlike ChatReads: get_chat_log's ledger
// read (agentusecase.ToolChatLogReader) is implemented by the agent CHAT concern
// (agentusecase.ChatUsecase.ReadChatLog), which does not exist yet at this point in
// construction — the exact chicken-and-egg agentusecase.New already resolves for
// Deps.Chats, Deps.ChatLogs and Deps.Lineage by binding the usecase to itself once
// built. See its doc comment.
//
// Metrics is wired here too but, unlike every port above, is deliberately
// ABSENT from the refusal switch: agentusecase.ToolMetrics is the one fail-OPEN
// dependency in Deps — losing the call counters is never a reason to fail
// daemon startup or narrow the tool surface, so there is nothing to refuse.
func newAgentToolDeps(
	minter *agentusecase.TokenMinter,
	repos *repositories.Container,
	review agentusecase.ToolReviewReader,
	threadBroadcast agentusecase.ToolThreadBroadcast,
	workspaces agentusecase.ToolWorkspaceBranchRenamer,
	browser agentusecase.ToolBrowserBridge,
) (agentusecase.ToolDeps, error) {
	switch {
	case minter == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no token minter")
	case workspaces == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no workspace usecase")
	case repos.AgentChat == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no chat store")
	case repos.Workspace == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no workspace store")
	case repos.ReviewThread == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no review thread store")
	case repos.ReviewThreadChanges == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no review thread change source")
	case review == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no branch review usecase")
	case threadBroadcast == nil:
		return agentusecase.ToolDeps{}, fmt.Errorf("usecases: wire agent tools: no thread broadcaster")
	}
	chatReader := agentChatReader{chats: repos.AgentChat}
	return agentusecase.ToolDeps{
		Scope:           repos.Workspace,
		Workspaces:      workspaces,
		Review:          review,
		Threads:         repos.ReviewThread,
		ThreadWrites:    repos.ReviewThread,
		ThreadChanges:   repos.ReviewThreadChanges,
		Idempotency:     agentusecase.NewToolIdempotency(),
		ThreadBroadcast: threadBroadcast,
		ChatReads:       chatReader,
		Browser:         browser,
		Metrics:         agentusecase.NewToolMetrics(),
	}, nil
}

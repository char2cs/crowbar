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
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
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

// agentWiring is the agentic surface as one value: the chat and tree usecases, plus the
// workspace reader the delete cascade reuses and the tool metrics nothing else can reach.
type agentWiring struct {
	chat     *agentusecase.Usecase
	chatTree agentusecase.TreeUsecase
	worktree worktreeResolver
	wsReader agentusecase.WorkspaceReader
	metrics  *agentusecase.ToolMetrics
}

// newAgentWiring assembles the agentic usecases in dependency order: the tree usecase
// holds the chat usecase, which tears chats down; the tree only decides which go.
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
		// The same adapter as the chat usecase's WorktreeCreator, so a cascading delete
		// and a failed branch-out's cleanup tear a workspace down through one call.
		worktreeChildCreator{worktree: workspaceUsecase},
		// And the census that gates it: Chat.WorkspaceID names the worktree a
		// chat is anchored to, never that it is anchored there alone, so the
		// delete asks who else is currently resolving to it before cascading.
		worktreeUsecase,
		// Home-scoped folders and chat placement go through Folder/Node.
		repos.Placement,
		repos.Placement,
	)
	// Runs under the chat's ledger writer, so it only starts the move's own pass.
	repos.ChatLedger.Subscribe(func(chatID string, _ []ledger.Row, st ledger.State) {
		if _, ready := st.MoveReady(); ready {
			chatTree.MoveWhenReady(context.Background(), chatID)
		}
	})
	return agentWiring{
		chat:     chat,
		chatTree: chatTree,
		worktree: worktreeUsecase,
		wsReader: wsReader,
		metrics:  toolDeps.Metrics,
	}, nil
}

// newAgentToolDeps refuses to return a partial tool surface: agenttools silently drops a
// tool whose port is nil, so a missing port must fail startup instead. Idempotency is
// built once to outlive per-request ToolSets; ChatLogs is bound later by agentusecase.New.
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

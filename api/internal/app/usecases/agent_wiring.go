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

type WorktreeResolver interface {
	Resolve(ctx context.Context, chatID string) (domain.Workspace, error)
}

type worktreeResolver struct {
	chats      worktree.ChatAncestryReader
	chatRows   worktree.ChatLister
	workspaces worktree.WorkspaceReader
	folders    worktree.Folders
	nodes      worktree.Nodes
}

func (r worktreeResolver) Resolve(
	ctx context.Context,
	chatID string,
) (domain.Workspace, error) {
	return worktree.Resolve(ctx, chatID, r.chats, r.workspaces)
}

func (r worktreeResolver) ChatsForWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]string, error) {
	return worktree.ChatsForWorkspace(ctx, workspaceID, r.chatRows, r.folders, r.nodes)
}

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
		Minter:                  minter,
		Tools:                   toolDeps,
		Snapshots:               chatSnapshots,
		Folders:                 repos.Placement,
		Nodes:                   repos.Placement,
		RepoRoots:               workspaceGitStatusReader{workspace: workspaceUsecase, repos: gormStores.Repositories},
		AgentWorking:            repos.SetAgentWorking,
		AnswerClock:             time.After,
	})
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
		worktreeChildCreator{worktree: workspaceUsecase},
		worktreeUsecase,
		repos.Placement,
		repos.Placement,
	)
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

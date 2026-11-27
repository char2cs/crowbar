package usecases

import (
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/app/usecases/git"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/discover"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/app/usecases/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/terminal"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

type GORMStores struct {
	Projects                 store.Store[domain.Project, string]
	Repositories             store.ScopedStore[domain.Repository, string]
	TerminalProfiles         store.Store[domain.TerminalProfile, string]
	TerminalSessions         store.Store[domain.TerminalSession, string]
	AgentProviderPreferences store.Store[domain.AgentProviderPreference, string]
	AgentPermissionDefault   store.Store[domain.AgentPermissionDefault, string]
	AgentModelManifestFetch  store.Store[domain.AgentModelManifestFetch, string]
	AgentChatTelemetry       store.Store[domain.AgentChatTelemetry, string]
}

type Container struct {
	Project              project.Usecase
	ProjectImport        project.ImportUsecase
	ProjectDelete        project.DeleteUsecase
	Workspace            workspace.Usecase
	File                 file.Usecase
	Git                  git.Usecase
	Terminal             terminal.Usecase
	ProviderSync         provider.Usecase
	BranchReview         branchreview.Usecase
	TerminalMeta         engineterminal.SessionMetaStore
	AgentChat            agentusecase.ChatUsecase
	AgentTurn            agentusecase.TurnUsecase
	AgentRunner          agentusecase.RunnerUsecase
	AgentAnswer          agentusecase.AnswerUsecase
	AgentProvider        agentusecase.ProviderUsecase
	AgentChatFolder      agentusecase.TreeUsecase
	AgentWorkspaceReader agentusecase.WorkspaceReader
	Worktree             WorktreeResolver

	agentToolMetrics *agentusecase.ToolMetrics
}

func (c *Container) AgentToolMetrics() map[string]agentusecase.ToolStat {
	return c.agentToolMetrics.Snapshot()
}

func New(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
	crowbarHome func() (string, error),
	threadBroadcast agentusecase.ToolThreadBroadcast,
	opts ...Option,
) (*Container, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	projectUsecase := project.New(
		gormStores.Projects,
		gormStores.Repositories,
		repos.Workspace,
		repos.Placement,
		repos.Placement,
		repos.AgentChat,
	)
	workspaceUsecase := workspace.New(
		repos.Workspace,
		engines.Git,
		projectUsecase,
		repos.Workspace,
		engines.Git,
		engines.Provider,
		gormStores.Repositories,
		nowFunc,
		crowbarHome,
		workspace.WithTerminalReaper(engines.Terminal),
		workspace.WithNodes(repos.Placement),
	)
	fileUsecase := file.New(newFsEngineAdapter(engines.FS), workspaceUsecase, crowbarHome)
	gitUsecase := git.New(
		engines.Git,
		workspaceUsecase,
	)
	providerSync := provider.New(
		repos.Workspace,
		engines.Provider,
	)
	projectImport := newProjectImport(repos, gormStores, engines, crowbarHome, projectUsecase)
	projectDelete := project.NewDelete(project.DeleteDeps{
		Projects:       gormStores.Projects,
		Repos:          gormStores.Repositories,
		Workspaces:     repos.Workspace,
		RepoWorkspaces: workspaceUsecase,
		Nodes:          repos.Placement,
		CrowbarHome:    crowbarHome,
	})
	branchReview := branchreview.New(
		repos.Workspace,
		repos.ReviewThread,
		gormStores.Repositories,
		engines.Git,
		nowFunc,
	)
	agentic, err := newAgentWiring(repos, gormStores, engines, crowbarHome, branchReview, threadBroadcast, workspaceUsecase, o.chatSnapshots, o.browser)
	if err != nil {
		return nil, err
	}
	workspaceUsecase.SetChatObserver(agentic.chat)
	owningChats := hierarchyOwningChats{tree: agentic.chatTree, chats: repos.AgentChat}
	workspaceUsecase.SetOwningChats(owningChats)
	projectImport.SetOwningChats(owningChats)
	worktreeUsecase := agentic.worktree
	terminalMeta := terminal.NewSessionMetaStore(
		worktreeUsecase,
		repos.Workspace,
		gormStores.TerminalSessions,
		crowbarHome,
	)
	terminalUsecase := terminal.New(
		engines.Terminal,
		gormStores.TerminalProfiles,
		worktreeUsecase,
		terminalMeta,
	)
	return &Container{
		Project:              projectUsecase,
		ProjectImport:        projectImport,
		ProjectDelete:        projectDelete,
		Workspace:            workspaceUsecase,
		File:                 fileUsecase,
		Git:                  gitUsecase,
		Terminal:             terminalUsecase,
		ProviderSync:         providerSync,
		BranchReview:         branchReview,
		TerminalMeta:         terminalMeta,
		AgentChat:            agentusecase.NewHomeCorrectedChats(agentic.chat, repos.Placement),
		AgentTurn:            agentic.chat,
		AgentRunner:          agentic.chat,
		AgentAnswer:          agentic.chat,
		AgentProvider:        agentic.chat,
		AgentChatFolder:      agentic.chatTree,
		AgentWorkspaceReader: agentic.wsReader,
		Worktree:             worktreeUsecase,
		agentToolMetrics:     agentic.metrics,
	}, nil
}

type Option func(*options)

type options struct {
	chatSnapshots *agentusecase.ChatSnapshots
	browser       agentusecase.ToolBrowserBridge
}

func WithBrowserBridge(b agentusecase.ToolBrowserBridge) Option {
	return func(o *options) { o.browser = b }
}

func WithChatSnapshots(s *agentusecase.ChatSnapshots) Option {
	return func(o *options) { o.chatSnapshots = s }
}

func newProjectImport(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
	crowbarHome func() (string, error),
	homeSlots project.HomeSlots,
) project.ImportUsecase {
	return project.NewImport(project.ImportDeps{
		Projects:    gormStores.Projects,
		Repos:       gormStores.Repositories,
		Workspaces:  repos.Workspace,
		Git:         engines.Git,
		Provider:    engines.Provider,
		Discover:    discover.Repos,
		RefRunner:   newRefRunner,
		Nodes:       repos.Placement,
		HomeSlots:   homeSlots,
		Now:         nowFunc,
		CrowbarHome: crowbarHome,
	})
}

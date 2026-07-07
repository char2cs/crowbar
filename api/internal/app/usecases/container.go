package usecases

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/app/usecases/git"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/discover"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/app/usecases/provider"
	"github.com/char2cs/crowbar/api/internal/app/usecases/terminal"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// GORMStores carries the plain-CRUD stores the usecases compose. It mirrors the
// app-layer GORMStores struct but lives here so the usecases package never
// imports its parent package (which would be an import cycle).
type GORMStores struct {
	Projects         store.Store[domain.Project, string]
	Repositories     store.Store[domain.Repository, string]
	TerminalProfiles store.Store[domain.TerminalProfile, string]
	TerminalSessions store.Store[domain.TerminalSession, string]
}

// Container holds every application usecase, composing the aggregate
// repositories, the GORM CRUD stores, and the engines.
type Container struct {
	Project       project.Usecase
	ProjectImport project.ImportUsecase
	ProjectDelete project.DeleteUsecase
	Workspace     workspace.Usecase
	File          file.Usecase
	Git           git.Usecase
	Terminal      terminal.Usecase
	ProviderSync  provider.Usecase
	Worktree      worktree.Usecase
	BranchReview  branchreview.Usecase
	// TerminalMeta is the durable session metadata store implementation exposed
	// so the API layer can inject it into the terminal engine via SetMetaStore
	// after both the engine and the usecase are constructed.
	TerminalMeta engineterminal.SessionMetaStore
	// Agent is the agentic-chat usecase: spawning vendor CLI segments, ingesting
	// their hooks through the context-move reducer, and serving the /v0/agent
	// REST surface.
	Agent *agent.Usecase
}

// New builds the usecases container. It takes the aggregate repositories, the
// GORM CRUD stores, and the engines rather than the app-layer GORMStores struct
// to keep the usecases package free of any dependency on its parent package.
// agentChats is the agentic-chat repository (built by the caller off the
// global view DB, next to the other GORM stores) and bc is the hub broadcaster
// the agent usecase pushes lifecycle events through (the app-layer *hub.Hub
// satisfies agent.Broadcaster).
func New(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
	crowbarHome func() (string, error),
	agentChats agentchat.Store,
	bc agent.Broadcaster,
) (*Container, error) {
	projectUsecase := project.New(
		gormStores.Projects,
		gormStores.Repositories,
	)
	workspaceUsecase := workspace.New(
		repos.Workspace,
		engines.Git,
		projectUsecase,
	)
	fileUsecase := file.New(
		newFsEngineAdapter(engines.FS),
		workspaceUsecase,
	)
	gitUsecase := git.New(
		engines.Git,
		workspaceUsecase,
	)
	terminalMeta := terminal.NewSessionMetaStore(
		repos.Workspace,
		gormStores.TerminalSessions,
		crowbarHome,
	)
	terminalUsecase := terminal.New(
		engines.Terminal,
		gormStores.TerminalProfiles,
		repos.Workspace,
		terminalMeta,
	)
	providerSync := provider.New(
		repos.Workspace,
		engines.Provider,
	)
	projectImport := project.NewImport(project.ImportDeps{
		Projects:    gormStores.Projects,
		Repos:       gormStores.Repositories,
		Workspaces:  repos.Workspace,
		Git:         engines.Git,
		Provider:    engines.Provider,
		Discover:    discover.Repos,
		RefRunner:   newRefRunner,
		Now:         nowFunc,
		CrowbarHome: crowbarHome,
	})
	projectDelete := project.NewDelete(project.DeleteDeps{
		Projects:    gormStores.Projects,
		Repos:       gormStores.Repositories,
		Workspaces:  repos.Workspace,
		Git:         engines.Git,
		CrowbarHome: crowbarHome,
	})
	worktreeUsecase := worktree.New(
		repos.Workspace,
		engines.Git,
		engines.Provider,
		gormStores.Repositories,
		nowFunc,
		crowbarHome,
		worktree.WithTerminalReaper(engines.Terminal),
	)
	branchReview := branchreview.New(
		repos.Workspace,
		repos.ReviewThread,
		repos.Chat,
		gormStores.Repositories,
		engines.Git,
		nowFunc,
	)
	agentUsecase := agent.New(
		agentChats,
		engineagent.NewRegistry(),
		engines.Terminal,
		bc,
		&agentWorkspaceReader{workspaces: repos.Workspace, crowbarHome: crowbarHome},
	)
	return &Container{
		Project:       projectUsecase,
		ProjectImport: projectImport,
		ProjectDelete: projectDelete,
		Workspace:     workspaceUsecase,
		File:          fileUsecase,
		Git:           gitUsecase,
		Terminal:      terminalUsecase,
		ProviderSync:  providerSync,
		Worktree:      worktreeUsecase,
		BranchReview:  branchReview,
		TerminalMeta:  terminalMeta,
		Agent:         agentUsecase,
	}, nil
}

// workspaceGetter is the minimal workspace-read surface agentWorkspaceReader
// needs: resolving the owning project/repo for a workspace id.
type workspaceGetter interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

// agentWorkspaceReader adapts the workspace repository into the agent
// usecase's WorkspaceReader seam (internal/app/usecases/agent.WorkspaceReader):
// given a workspace id, it resolves the owning project/repo from the
// workspace-location index and derives the git worktree directory via the same
// worktreepath helper the worktree usecase uses.
//
// It shares the container's injected crowbarHome resolver (the same one every
// other path-deriving usecase here is built with) rather than reading
// metadata.GetHomePath() directly: path-deriving usecases MUST resolve against
// the home the adapter container was actually opened against (see
// adapter.Container.CrowbarHome's doc comment), and a hermetic test that
// overrides the home via adapter.WithHomeDir (rather than the CROWBAR_HOME env
// var) would otherwise silently diverge.
type agentWorkspaceReader struct {
	workspaces  workspaceGetter
	crowbarHome func() (string, error)
}

// WorktreeDir implements agent.WorkspaceReader.
func (r *agentWorkspaceReader) WorktreeDir(
	ctx context.Context,
	workspaceID string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	home, err := r.crowbarHome()
	if err != nil {
		return "", "", "", "", fmt.Errorf("usecases: agent workspace reader: crowbar home: %w", err)
	}
	w, err := r.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", "", "", "", fmt.Errorf("usecases: agent workspace reader: get workspace: %w", err)
	}
	return home, w.ProjectID, w.RepoID, worktreepath.For(home, w.ProjectID, w.RepoID, workspaceID), nil
}

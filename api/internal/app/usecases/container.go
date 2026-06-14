package usecases

import (
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat"
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
)

// GORMStores carries the plain-CRUD stores the usecases compose. It mirrors the
// app-layer GORMStores struct but lives here so the usecases package never
// imports its parent package (which would be an import cycle).
type GORMStores struct {
	Projects         store.Store[domain.Project, string]
	Repositories     store.Store[domain.Repository, string]
	TerminalProfiles store.Store[domain.TerminalProfile, string]
}

// Container holds every application usecase, composing the aggregate
// repositories, the GORM CRUD stores, and the engines.
type Container struct {
	Project       project.Usecase
	ProjectImport project.ImportUsecase
	ProjectDelete project.DeleteUsecase
	Workspace     workspace.Usecase
	Chat          chat.Usecase
	File          file.Usecase
	Git           git.Usecase
	Terminal      terminal.Usecase
	ProviderSync  provider.Usecase
	Worktree      worktree.Usecase
	BranchReview  branchreview.Usecase
}

// New builds the usecases container. It takes the aggregate repositories, the
// GORM CRUD stores, and the engines rather than the app-layer GORMStores struct
// to keep the usecases package free of any dependency on its parent package.
func New(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
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
	chatUsecase := chat.New(
		repos.Chat,
		repos.Workspace,
		projectUsecase,
		nowFunc,
	)
	fileUsecase := file.New(
		newFsEngineAdapter(engines.FS),
		workspaceUsecase,
	)
	gitUsecase := git.New(
		engines.Git,
		workspaceUsecase,
	)
	terminalUsecase := terminal.New(
		engines.Terminal,
		gormStores.TerminalProfiles,
		repos.Workspace,
	)
	providerSync := provider.New(
		repos.Workspace,
		engines.Provider,
	)
	projectImport := project.NewImport(project.ImportDeps{
		Projects:   gormStores.Projects,
		Repos:      gormStores.Repositories,
		Workspaces: repos.Workspace,
		Git:        engines.Git,
		Provider:   engines.Provider,
		Discover:   discover.Repos,
		RefRunner:  newRefRunner,
		Now:        nowFunc,
	})
	projectDelete := project.NewDelete(project.DeleteDeps{
		Projects:    gormStores.Projects,
		Repos:       gormStores.Repositories,
		Workspaces:  repos.Workspace,
		Git:         engines.Git,
		CrowbarHome: worktreepath.DefaultCrowbarHome,
	})
	worktreeUsecase := worktree.New(
		repos.Workspace,
		engines.Git,
		engines.Provider,
		gormStores.Repositories,
		nowFunc,
		worktreepath.DefaultCrowbarHome,
	)
	branchReview := branchreview.New(
		repos.Workspace,
		repos.ReviewThread,
		repos.Chat,
		gormStores.Repositories,
		engines.Git,
		nowFunc,
	)
	return &Container{
		Project:       projectUsecase,
		ProjectImport: projectImport,
		ProjectDelete: projectDelete,
		Workspace:     workspaceUsecase,
		Chat:          chatUsecase,
		File:          fileUsecase,
		Git:           gitUsecase,
		Terminal:      terminalUsecase,
		ProviderSync:  providerSync,
		Worktree:      worktreeUsecase,
		BranchReview:  branchReview,
	}, nil
}

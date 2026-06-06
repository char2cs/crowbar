package usecases

import (
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/discover"
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
	Project       ProjectUsecase
	ProjectImport ProjectImportUsecase
	Workspace     WorkspaceUsecase
	Chat          ChatUsecase
	File          FileUsecase
	Git           GitUsecase
	Terminal      TerminalUsecase
	ProviderSync  ProviderSyncUsecase
	Worktree      WorktreeUsecase
	BranchReview  BranchReviewUsecase
}

// New builds the usecases container. It takes the aggregate repositories, the
// GORM CRUD stores, and the engines rather than the app-layer GORMStores struct
// to keep the usecases package free of any dependency on its parent package.
func New(
	repos *repositories.Container,
	gormStores GORMStores,
	engines *engine.Container,
) (*Container, error) {
	project := NewProjectUsecase(
		gormStores.Projects,
		gormStores.Repositories,
	)
	workspace := NewWorkspaceUsecase(
		repos.Workspace,
		engines.Git,
		project,
	)
	chat := NewChatUsecase(
		repos.Chat,
		repos.Workspace,
		project,
		nowFunc,
	)
	file := NewFileUsecase(
		newFsEngineAdapter(engines.FS),
		workspace,
	)
	git := NewGitUsecase(
		engines.Git,
		workspace,
	)
	terminal := NewTerminalUsecase(
		engines.Terminal,
		gormStores.TerminalProfiles,
		repos.Workspace,
	)
	providerSync := NewProviderSyncUsecase(
		repos.Workspace,
		engines.Provider,
	)
	projectImport := NewProjectImport(ProjectImportDeps{
		Projects:   gormStores.Projects,
		Repos:      gormStores.Repositories,
		Workspaces: repos.Workspace,
		Git:        engines.Git,
		Provider:   engines.Provider,
		Discover:   discover.Repos,
		RefRunner:  newRefRunner,
		Now:        nowFunc,
	})
	worktree := NewWorktreeUsecase(
		repos.Workspace,
		engines.Git,
		engines.Provider,
		gormStores.Repositories,
		nowFunc,
	)
	branchReview := NewBranchReviewUsecase(
		repos.Workspace,
		repos.ReviewThread,
		repos.Chat,
		gormStores.Repositories,
		engines.Git,
		nowFunc,
	)
	return &Container{
		Project:       project,
		ProjectImport: projectImport,
		Workspace:     workspace,
		Chat:          chat,
		File:          file,
		Git:           git,
		Terminal:      terminal,
		ProviderSync:  providerSync,
		Worktree:      worktree,
		BranchReview:  branchReview,
	}, nil
}

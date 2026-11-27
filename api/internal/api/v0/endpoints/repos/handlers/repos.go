package handlers

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

type Store interface {
	FindAll(
		ctx context.Context,
	) ([]domain.Repository, error)
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Repository, error)
	Save(
		ctx context.Context,
		repo domain.Repository,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
}

type BranchProviderEngine interface {
	ProtectedBranches(ctx context.Context, repoPath string) ([]string, error)
	OpenPullRequests(ctx context.Context, repoPath string) ([]providertypes.PRLink, error)
}

type WorkspaceReader interface {
	List(ctx context.Context) ([]domain.Workspace, error)
}

type RepoDeleter interface {
	BeginRepoDelete(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) (domain.Repository, error)
	DeleteRepo(
		ctx context.Context,
		repo domain.Repository,
		consent domain.DeleteConsent,
	) error
}

type RemoteRefresher interface {
	FetchPrune(ctx context.Context, repoPath string) error
	Branches(ctx context.Context, repoPath string) ([]gitdomain.Branch, error)
}

type BranchEntry struct {
	Name         string `json:"name"`
	IsProtected  bool   `json:"isProtected"`
	HasWorkspace bool   `json:"hasWorkspace"`
}

type AvatarBytesFetcher func(
	ctx context.Context,
	repoPath string,
) ([]byte, string, error)

type RepoImporter interface {
	ImportRepo(
		ctx context.Context,
		in project.RepoImport,
		progress project.RepoProgress,
	) (domain.Repository, error)
	CheckRepoImportable(
		ctx context.Context,
		projectID string,
		repoPath string,
	) error
}

type RepoUpdater interface {
	UpdateRepo(
		ctx context.Context,
		repoID string,
		in project.RepoUpdate,
	) error
}

type Operations interface {
	Start(
		ctx context.Context,
		spec operation.Spec,
		work operation.Work,
	) (domain.Operation, error)
	libs.Replayer
}

type Handlers struct {
	store       Store
	provider    BranchProviderEngine
	wsReader    WorkspaceReader
	deleter     RepoDeleter
	remote      RemoteRefresher
	importer    RepoImporter
	updater     RepoUpdater
	placements  *dto.Placements
	crowbarHome func() (string, error)
	fetchAvatar AvatarBytesFetcher
	stat        func(string) (os.FileInfo, error)
	ops         Operations
}

func New(
	store Store,
	crowbarHome func() (string, error),
) *Handlers {
	if crowbarHome == nil {
		panic("repos: crowbarHome is required")
	}
	h := &Handlers{
		store:       store,
		crowbarHome: crowbarHome,
		fetchAvatar: fetchGithubAvatarBytes,
		stat:        os.Stat,
	}
	return h
}

func NewWithDeps(
	store Store,
	prov BranchProviderEngine,
	wsReader WorkspaceReader,
	ops Operations,
	crowbarHome func() (string, error),
	placements *dto.Placements,
) *Handlers {
	h := New(store, crowbarHome)
	h.placements = placements
	h.provider = prov
	h.wsReader = wsReader
	h.ops = ops
	return h
}

func (h *Handlers) WithRepoDeleter(
	deleter RepoDeleter,
) *Handlers {
	h.deleter = deleter
	return h
}

func (h *Handlers) WithRemoteRefresher(
	remote RemoteRefresher,
) *Handlers {
	if remote != nil {
		h.remote = remote
	}
	return h
}

func (h *Handlers) WithImporter(
	importer RepoImporter,
) *Handlers {
	if importer != nil {
		h.importer = importer
	}
	return h
}

func (h *Handlers) WithUpdater(
	updater RepoUpdater,
) *Handlers {
	if updater != nil {
		h.updater = updater
	}
	return h
}

func (h *Handlers) WithStat(
	stat func(string) (os.FileInfo, error),
) *Handlers {
	if stat != nil {
		h.stat = stat
	}
	return h
}

func (h *Handlers) placementOf(
	ctx context.Context,
	repoID string,
) (dto.RepoPlacement, error) {
	if h.placements == nil {
		return dto.RepoPlacement{}, nil
	}
	p, err := h.placements.At(ctx, repoID)
	if err != nil {
		return dto.RepoPlacement{}, fmt.Errorf("repos: %w", err)
	}
	return p, nil
}

func (h *Handlers) placementsOf(
	ctx context.Context,
	repos []domain.Repository,
) map[string]dto.RepoPlacement {
	if h.placements == nil {
		return nil
	}
	return h.placements.Of(ctx, repos)
}

func (h *Handlers) List(
	c *gin.Context,
) {
	repos, err := h.store.FindAll(c.Request.Context())
	if err != nil {
		libs.Fail(c, err)
		return
	}
	projectID := c.Param("projectId")
	if projectID == "" {
		projectID = c.Query("projectId")
	}
	repos = filterByProject(repos, projectID)
	libs.WriteQueryOK(c, dto.RepoDTOList(repos, h.placementsOf(c.Request.Context(), repos)))
}

func (h *Handlers) Detail(
	c *gin.Context,
) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil {
		libs.Fail(c, err)
		return
	}
	if repo == nil {
		libs.Fail(c, problem.ErrNotFound)
		return
	}
	placement, err := h.placementOf(c.Request.Context(), repo.ID)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, dto.RepoDTOFrom(*repo, placement))
}

func filterByProject(
	repos []domain.Repository,
	projectID string,
) []domain.Repository {
	if projectID == "" {
		return repos
	}
	filtered := make([]domain.Repository, 0, len(repos))
	for _, r := range repos {
		if r.ProjectID == projectID {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

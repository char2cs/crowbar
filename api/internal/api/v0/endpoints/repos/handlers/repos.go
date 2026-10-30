// Package handlers holds the gin handlers backing the repos endpoint.
package handlers

import (
	"context"
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

// Store is the full surface the repos handlers need over the repository GORM
// table: list every repo, fetch one by id, persist a new one, and remove one.
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

// BranchProviderEngine is the provider surface the Branches and PullRequests
// handlers need.
type BranchProviderEngine interface {
	ProtectedBranches(ctx context.Context, repoPath string) ([]string, error)
	OpenPullRequests(ctx context.Context, repoPath string) ([]providertypes.PRLink, error)
}

// WorkspaceReader is the workspace surface the Branches handler needs.
type WorkspaceReader interface {
	List(ctx context.Context) ([]domain.Workspace, error)
}

// RepoDeleter removes a repo and everything it owns through the one delete
// lifecycle (project.DeleteUsecase.DeleteRepo): workspaces retired first, then
// the row, its Node row and its entity directory. The handler only binds HTTP.
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

// RemoteRefresher is the narrow git surface the Branches handler uses to make
// its listing reflect the remote as it is now rather than as the clone last
// heard it. It goes through the git engine (not a bare shell-out) so the fetch
// takes the same per-clone lock every other git operation does.
type RemoteRefresher interface {
	FetchPrune(ctx context.Context, repoPath string) error
	Branches(ctx context.Context, repoPath string) ([]gitdomain.Branch, error)
}

// BranchEntry is one item in the GET /v0/projects/:projectId/repos/:repoId/branches response.
type BranchEntry struct {
	Name         string `json:"name"`
	IsProtected  bool   `json:"isProtected"`
	HasWorkspace bool   `json:"hasWorkspace"`
}

// AvatarBytesFetcher downloads the repo owner's avatar image bytes plus the
// response content-type, best-effort: (nil, "", nil) on absence.
type AvatarBytesFetcher func(
	ctx context.Context,
	repoPath string,
) ([]byte, string, error)

// RepoImporter runs the full per-repo import for a single repo path under an
// already-persisted project: it persists the Repository, writes the repo icon
// (local icon or best-effort GitHub owner avatar), adopts the default/protected
// branches as workspaces, and returns the created repo (00 §14 Step 3).
type RepoImporter interface {
	ImportRepo(
		ctx context.Context,
		in project.RepoImport,
		progress project.RepoProgress,
	) (domain.Repository, error)
	// CheckRepoImportable reports whether repoPath may be imported under
	// projectID — it refuses a folder another project has already added. Create
	// runs it before the 202 so the refusal answers the request.
	CheckRepoImportable(
		ctx context.Context,
		projectID string,
		repoPath string,
	) error
}

// NodeReader is the narrow Node surface the repos handlers need: reading a
// RepoDTO's own sidebar position (Order/FolderID — see domain.Node and
// dto.RepoPlacement), and forgetting the row when the repo goes. Every other
// WRITE to a repo's position goes through RepoUpdater
// (project.Usecase.UpdateRepo) instead.
type NodeReader interface {
	Forget(
		ctx context.Context,
		id string,
	) error
	GetNode(
		ctx context.Context,
		id string,
	) (domain.Node, error)
}

// RepoUpdater applies a partial repository update — display name (and its
// derived avatar), sidebar order, owning project — and returns the updated repo
// so the handler can broadcast the new RepoDTO.
type RepoUpdater interface {
	UpdateRepo(
		ctx context.Context,
		repoID string,
		in project.RepoUpdate,
	) (project.RepoUpdated, error)
}

// Operations runs the work a create or a delete hands off after answering 202.
type Operations interface {
	Start(
		ctx context.Context,
		spec operation.Spec,
		work operation.Work,
	) (domain.Operation, error)
}

// Handlers serves the /v0/repos routes from the repository GORM store. A
// create or a delete checks what it can, answers 202 with its operation, and
// delivers the resulting RepoDTO on the Repos stream.
type Handlers struct {
	store       Store
	provider    BranchProviderEngine
	wsReader    WorkspaceReader
	deleter     RepoDeleter
	remote      RemoteRefresher
	importer    RepoImporter
	updater     RepoUpdater
	nodes       NodeReader
	crowbarHome func() (string, error)
	fetchAvatar AvatarBytesFetcher
	broadcast   func(dto.RepoDTO)
	stat        func(string) (os.FileInfo, error)
	ops         Operations
}

// New builds the repos Handlers from the repository GORM store and the
// crowbar-home resolver the icon handlers store and serve through, which is
// required. The broadcast is a no-op until NewWithDeps wires the fan-out.
func New(
	store Store,
	crowbarHome func() (string, error),
) *Handlers {
	if crowbarHome == nil {
		panic("repos: crowbarHome is required")
	}
	return &Handlers{
		store:       store,
		crowbarHome: crowbarHome,
		fetchAvatar: fetchGithubAvatarBytes,
		broadcast:   func(dto.RepoDTO) {},
		stat:        os.Stat,
	}
}

// NewWithDeps builds Handlers with the optional provider + workspace deps
// needed for the Branches endpoint, the operations service create and delete
// run under, and the Repos-channel broadcast func. A nil broadcast degrades to
// a no-op.
func NewWithDeps(
	store Store,
	prov BranchProviderEngine,
	wsReader WorkspaceReader,
	ops Operations,
	broadcast func(dto.RepoDTO),
	crowbarHome func() (string, error),
) *Handlers {
	h := New(store, crowbarHome)
	h.provider = prov
	h.wsReader = wsReader
	h.ops = ops
	if broadcast != nil {
		h.broadcast = broadcast
	}
	return h
}

// WithRepoDeleter wires the delete lifecycle DeleteRepo runs.
func (h *Handlers) WithRepoDeleter(
	deleter RepoDeleter,
) *Handlers {
	h.deleter = deleter
	return h
}

// WithRemoteRefresher wires the git surface the Branches handler uses to
// refresh origin before listing. A nil arg leaves the handler serving the
// clone's cached remote-tracking refs (tests, and any wiring without a git
// engine).
func (h *Handlers) WithRemoteRefresher(
	remote RemoteRefresher,
) *Handlers {
	if remote != nil {
		h.remote = remote
	}
	return h
}

// WithImporter wires the full repo-import usecase that Create runs in the
// background so adding a repo auto-adopts the default/protected-branch
// workspaces and seeds the GitHub avatar (00 §14 Step 3). A nil arg leaves the
// handler on its bare buildRepo+Save fallback.
func (h *Handlers) WithImporter(
	importer RepoImporter,
) *Handlers {
	if importer != nil {
		h.importer = importer
	}
	return h
}

// WithUpdater wires the repo-update usecase the Patch handler calls to change a
// repo's name, sidebar order or owning project. A nil arg leaves the PATCH
// unavailable (the handler answers 500), matching the bare-handler fallback of
// WithImporter.
func (h *Handlers) WithUpdater(
	updater RepoUpdater,
) *Handlers {
	if updater != nil {
		h.updater = updater
	}
	return h
}

// WithNodes wires the read-only Node surface a RepoDTO reads its own sidebar
// position from (see NodeReader). A nil arg leaves every RepoDTO's Order/
// FolderID at the zero value (root, order 0) — the same degrade a repo with no
// Node row yet renders as.
func (h *Handlers) WithNodes(
	nodes NodeReader,
) *Handlers {
	if nodes != nil {
		h.nodes = nodes
	}
	return h
}

// WithStat overrides the filesystem stat used to validate the create path
// synchronously. Intended for tests; a nil arg leaves os.Stat in place.
func (h *Handlers) WithStat(
	stat func(string) (os.FileInfo, error),
) *Handlers {
	if stat != nil {
		h.stat = stat
	}
	return h
}

// placementOf resolves repoID's own sidebar position from its Node row,
// degrading to the zero value (root, order 0) when no reader is wired or no
// row exists yet — a repo created through the bare buildRepo+Save fallback (no
// importer wired) never gets one, and every RepoDTO must still render. This is
// display-only: never fails the request it enriches.
func (h *Handlers) placementOf(
	ctx context.Context,
	repoID string,
) dto.RepoPlacement {
	if h.nodes == nil {
		return dto.RepoPlacement{}
	}
	n, err := h.nodes.GetNode(ctx, repoID)
	if err != nil {
		return dto.RepoPlacement{}
	}
	return dto.RepoPlacement{FolderID: n.ParentID, Order: n.Order}
}

// placementsOf resolves every repo's own position in one pass, for the List/
// snapshot paths — see placementOf.
func (h *Handlers) placementsOf(
	ctx context.Context,
	repos []domain.Repository,
) map[string]dto.RepoPlacement {
	placements := make(map[string]dto.RepoPlacement, len(repos))
	for _, r := range repos {
		placements[r.ID] = h.placementOf(ctx, r.ID)
	}
	return placements
}

// List handles GET /v0/repos, returning every repo as RepoDTO[]. The optional
// projectId query parameter filters the result to one project's repos.
func (h *Handlers) List(
	c *gin.Context,
) {
	repos, err := h.store.FindAll(c.Request.Context())
	if err != nil {
		libs.Fail(c, err)
		return
	}
	// The route is hierarchical, so the project is a PATH parameter. Reading it
	// from the query string (as this did) meant the filter never ran: the value
	// was always empty, `filterByProject` returned everything, and every project
	// listed every repo in the install. One project hides it completely — the
	// unfiltered answer and the correct one are the same list — which is why it
	// survived until a second project existed.
	projectID := c.Param("projectId")
	if projectID == "" {
		projectID = c.Query("projectId")
	}
	repos = filterByProject(repos, projectID)
	libs.WriteQueryOK(c, dto.RepoDTOList(repos, h.placementsOf(c.Request.Context(), repos)))
}

// Detail handles GET /v0/projects/:projectId/repos/:repoId, returning a single RepoDTO. The workspace
// tree is not yet composed by any usecase, so detail carries the repo fields
// only.
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
	libs.WriteQueryOK(c, dto.RepoDTOFrom(*repo, h.placementOf(c.Request.Context(), repo.ID)))
}

// filterByProject keeps only the repos whose ProjectID matches projectID; an
// empty projectID returns the input unchanged.
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

// Package handlers holds the gin handlers backing the repos endpoint.
package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/icons"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
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
	BeginRepoDelete(ctx context.Context, repo domain.Repository) (domain.Repository, error)
	DeleteRepo(ctx context.Context, repo domain.Repository) error
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
		projectID string,
		name string,
		repoPath string,
	) (domain.Repository, error)
	// CheckRepoImportable reports whether repoPath may be imported under
	// projectID — it refuses a folder another project has already added. Create
	// runs it BEFORE the 202 so the refusal is an HTTP error the add-repo dialog
	// can show, rather than a background failure the client waits out.
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

// Handlers serves the /v0/repos routes from the repository GORM store. Domain
// mutations (create, delete) follow the fail-fast/good-path-async pattern
// (00 §4): validate synchronously, return 202, run the slow work in the
// background, then deliver the resulting RepoDTO on the Repos WebSocket stream
// via broadcast.
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
	// async tracks the detached runAsync ops so callers can block on their real
	// completion instead of guessing with a sleep (see runAsync / WaitAsync).
	async sync.WaitGroup
}

// New builds the repos Handlers from the repository GORM store. The broadcast
// func is the Repos-channel fan-out; a nil broadcast degrades to a no-op so the
// handler never panics when wired without a hub (tests).
func New(
	store Store,
) *Handlers {
	return &Handlers{
		store:       store,
		crowbarHome: icons.DefaultCrowbarHome,
		fetchAvatar: fetchGithubAvatarBytes,
		broadcast:   func(dto.RepoDTO) {},
		stat:        os.Stat,
	}
}

// NewWithDeps builds Handlers with the optional provider + workspace deps
// needed for the Branches endpoint plus the Repos-channel broadcast func. A nil
// broadcast degrades to a no-op.
func NewWithDeps(
	store Store,
	prov BranchProviderEngine,
	wsReader WorkspaceReader,
	broadcast func(dto.RepoDTO),
) *Handlers {
	if broadcast == nil {
		broadcast = func(dto.RepoDTO) {}
	}
	return &Handlers{
		store:       store,
		provider:    prov,
		wsReader:    wsReader,
		crowbarHome: icons.DefaultCrowbarHome,
		fetchAvatar: fetchGithubAvatarBytes,
		broadcast:   broadcast,
		stat:        os.Stat,
	}
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

// WithIconStorage overrides the crowbar-home resolver and the GitHub avatar
// fetcher used by the icon handlers. Intended for tests; both nil args leave
// the defaults in place.
func (h *Handlers) WithIconStorage(
	home func() (string, error),
	fetch AvatarBytesFetcher,
) *Handlers {
	if home != nil {
		h.crowbarHome = home
	}
	if fetch != nil {
		h.fetchAvatar = fetch
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	if repo == nil {
		status, msg := libs.StatusAndMessage(apperr.ErrNotFound)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.RepoDTOFrom(*repo, h.placementOf(c.Request.Context(), repo.ID)))
}

// createRequest is the POST .../repos body.
type createRequest struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
}

// Create handles POST /v0/projects/:projectId/repos. It validates the request
// synchronously (body shape, name present, path present and existing on disk)
// returning 4xx on any failure; then it returns 202 and persists the repository
// in the background. On success the created repo is delivered as a RepoDTO on
// the Repos WebSocket stream. The defaultBranch field is optional: when omitted
// the background work derives it from the local git repository at path via
// symbolic-ref HEAD.
func (h *Handlers) Create(
	c *gin.Context,
) {
	var body createRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	// The URL :projectId is authoritative — the repo is created under the project
	// in the path. A body-supplied projectId must never override it (that would let
	// a POST to /projects/A/repos create a repo under project B). The body field is
	// ignored.
	body.ProjectID = c.Param("projectId")
	if body.Name == "" {
		libs.WriteErr(c, http.StatusBadRequest, "name is required")
		return
	}
	if !safeRepoName(body.Name) {
		libs.WriteErr(c, http.StatusBadRequest, unsafeRepoNameMessage)
		return
	}
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	if _, err := h.stat(body.Path); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "path does not exist")
		return
	}
	// A folder another project already owns is refused here, synchronously: the
	// import itself runs after the 202, where its only channel back to the client
	// is a broadcast that never comes — the dialog would sit on a 30s wait and
	// then blame a timeout for what is a plain, answerable conflict.
	if h.importer == nil {
		libs.WriteErr(c, http.StatusInternalServerError, "repo import is not wired")
		return
	}
	if err := h.importer.CheckRepoImportable(c.Request.Context(), body.ProjectID, body.Path); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(c.Request.Context(), func(ctx context.Context) {
		repo, ok := h.persistRepo(ctx, body)
		if !ok {
			return
		}
		h.broadcast(dto.RepoDTOFrom(repo, h.placementOf(ctx, repo.ID)))
	})
}

// persistRepo runs the background create work: the complete import
// (default-branch workspace adoption, protected-branch rows, GitHub avatar).
// ok is false when it failed and no RepoDTO should be broadcast.
func (h *Handlers) persistRepo(
	ctx context.Context,
	body createRequest,
) (domain.Repository, bool) {
	repo, err := h.importer.ImportRepo(ctx, body.ProjectID, body.Name, body.Path)
	if err != nil {
		slog.ErrorContext(ctx, "create repo: import", "path", body.Path, "err", err)
		return domain.Repository{}, false
	}
	return repo, true
}

// patchRequest is the PATCH .../repos/:repoId body. Every field is optional and
// a nil field is left as it is. ProjectID may only name the repo's own project.
type patchRequest struct {
	Name      *string `json:"name"`
	ProjectID *string `json:"projectId"`
	Order     *int    `json:"order"`
	// FolderID re-files the repo's own entry within its project's home tree —
	// "" for the project's home root, a project-home folder id otherwise.
	// *string, not string, for the same reason ProjectID is: a request that
	// omits it must leave the repo's current folder untouched, not clear it.
	FolderID *string `json:"folderId"`
}

// unsafeRepoNameMessage is the 400 both name-taking endpoints answer with.
const unsafeRepoNameMessage = "name must not contain a path separator or be a dot-only component"

// safeRepoName reports whether a user-supplied repository name may be used as
// an on-disk identity.
//
// The name is not only a label. A repository with no parseable git remote falls
// back to it for its worktree slug (worktreepath.RemoteSlug), which is joined
// into <home>/projects/<project>/<slug>/<branch>/worktree — and filepath.Join
// CLEANS what it joins, so "../../../../tmp/pwned" derives worktrees outside the
// crowbar home. Workspaces created out there can never be reclaimed: every
// removal guard refuses to touch anything that is not strictly under home.
//
// This is newly reachable. The name used to be filepath.Base(repoPath), which
// cannot contain a separator; now both the create and the rename endpoint take
// it verbatim from the client, so both check it here — before it is persisted,
// rather than at each of the many places it is later joined.
func safeRepoName(
	name string,
) bool {
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	return strings.Trim(name, ".") != ""
}

// Patch handles PATCH /v0/projects/:projectId/repos/:repoId: rename, sidebar
// reorder and re-filing into a home folder. It delivers the updated repo as a
// RepoDTO on the repos WebSocket stream so every client's sidebar refreshes.
//
// Validation is synchronous, and so is the write: none of the three is a git
// operation, so unlike Create this runs inline and answers 204 — the updated
// avatar and order ride the broadcast, not this response (the FE apiFetch throws
// on any non-enveloped 200 body, matching the icon mutations).
//
// A projectId naming another project is refused with 409: a repo's project is
// fixed at import (project.refuseProjectMove).
func (h *Handlers) Patch(
	c *gin.Context,
) {
	if h.updater == nil {
		libs.WriteErr(c, http.StatusInternalServerError, "repo update unavailable")
		return
	}
	var body patchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	update, ok := h.bindRepoUpdate(c, body)
	if !ok {
		return
	}
	updated, err := h.updater.UpdateRepo(c.Request.Context(), c.Param("repoId"), update)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	// The DECIDED placement, never a re-read: the Node projection folds after
	// the write returns, so a read here can still serve the old order.
	h.broadcast(dto.RepoDTOFrom(updated.Repo, dto.RepoPlacement{FolderID: updated.Node.ParentID, Order: updated.Node.Order}))
	h.broadcastShiftedRepos(c.Request.Context(), updated.Shifted)
	c.Status(http.StatusNoContent)
}

// broadcastShiftedRepos announces every OTHER repo a placement renumbered as
// collateral; chat/folder rows ride their own channel.
func (h *Handlers) broadcastShiftedRepos(
	ctx context.Context,
	shifted []domain.Node,
) {
	for _, n := range shifted {
		if n.Kind != domain.NodeKindRepo {
			continue
		}
		repo, err := h.store.FindByKey(ctx, n.ID)
		if err != nil || repo == nil {
			continue
		}
		h.broadcast(dto.RepoDTOFrom(*repo, dto.RepoPlacement{FolderID: n.ParentID, Order: n.Order}))
	}
}

// bindRepoUpdate validates the PATCH body into a project.RepoUpdate, writing the
// 400 and returning ok=false on any rejection. The name rules are the SAME ones
// create enforces, and for the same reason: a repo with no parseable remote falls
// back to its name for the on-disk worktree slug, so a separator or a dot-only
// component would derive worktrees outside the crowbar home.
func (h *Handlers) bindRepoUpdate(
	c *gin.Context,
	body patchRequest,
) (project.RepoUpdate, bool) {
	update := project.RepoUpdate{ProjectID: body.ProjectID, Order: body.Order, FolderID: body.FolderID}
	if body.Name == nil {
		return update, true
	}
	name := strings.TrimSpace(*body.Name)
	if name == "" {
		libs.WriteErr(c, http.StatusBadRequest, "name is required")
		return project.RepoUpdate{}, false
	}
	if !safeRepoName(name) {
		libs.WriteErr(c, http.StatusBadRequest, unsafeRepoNameMessage)
		return project.RepoUpdate{}, false
	}
	update.Name = &name
	return update, true
}

// DeleteRepo handles DELETE /v0/projects/:projectId/repos/:repoId. It validates
// the repo exists synchronously (4xx if not), then returns 202 and runs the
// removal in the background through the one delete lifecycle
// (RepoDeleter.DeleteRepo), broadcasting the deleted-status RepoDTO tombstone
// once it is done. A failure is never silent: the repo is re-broadcast as still
// present, carrying the LastError the usecase recorded, and boot resumes the
// delete. The user's real repository directory (repo.Path) is never touched.
func (h *Handlers) DeleteRepo(
	c *gin.Context,
) {
	projectID := c.Param("projectId")
	repoID := c.Param("repoId")
	repo, err := h.store.FindByKey(c.Request.Context(), repoID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	if repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	if h.deleter == nil {
		libs.WriteErr(c, http.StatusInternalServerError, "repo delete is not wired")
		return
	}
	// The intent is durable, and a previous attempt's error cleared on every
	// client, before the 202: from here on boot finishes what this starts.
	marked, err := h.deleter.BeginRepoDelete(c.Request.Context(), *repo)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	h.broadcast(dto.RepoDTOFrom(marked, h.placementOf(c.Request.Context(), repoID)))
	libs.WriteAccepted(c)
	h.runAsync(c.Request.Context(), func(ctx context.Context) {
		if err := h.deleter.DeleteRepo(ctx, marked); err != nil {
			slog.ErrorContext(ctx, "delete repo: stopped; the repo stays", "repo", repoID, "err", err)
			if row, getErr := h.store.FindByKey(ctx, repoID); getErr == nil && row != nil {
				h.broadcast(dto.RepoDTOFrom(*row, h.placementOf(ctx, repoID)))
			}
			return
		}
		h.broadcast(dto.RepoDTO{ID: repoID, ProjectID: projectID, Status: "deleted"})
	})
}

// Icon handles GET /v0/projects/:projectId/repos/:repoId/icon. It serves the
// on-disk icon bytes stored at worktreepath.RepoIconPath, sniffing the
// content-type from the bytes. Returns 404 when the repo has no on-disk icon.
func (h *Handlers) Icon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil || !repo.AvatarHasIcon {
		c.Status(http.StatusNotFound)
		return
	}
	iconPath, ok := h.iconPath(c)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	icons.Serve(c, iconPath)
}

// iconPath resolves the entity-scoped icon file path from the request's
// :projectId/:repoId params and the configured crowbar home. ok is false when
// the home cannot be resolved.
func (h *Handlers) iconPath(
	c *gin.Context,
) (string, bool) {
	home, err := h.crowbarHome()
	if err != nil || home == "" {
		return "", false
	}
	return repoIconPath(home, c.Param("projectId"), c.Param("repoId")), true
}

// repoIconPath mirrors worktreepath.RepoIconPath without importing the
// usecase-internal package (forbidden from the api layer):
// <crowbarHome>/projects/<projectId>/<repoId>/icon.
func repoIconPath(
	crowbarHome string,
	projectID string,
	repoID string,
) string {
	return filepath.Join(crowbarHome, "projects", projectID, repoID, "icon")
}

// fetchGithubAvatarBytes resolves the repo owner avatar URL via git + gh and
// downloads its bytes. Best-effort: returns (nil, "", nil) on any soft
// failure (no origin, no gh auth, transport error).
func fetchGithubAvatarBytes(
	ctx context.Context,
	repoPath string,
) ([]byte, string, error) {
	url := githubAvatarURL(ctx, repoPath)
	if url == "" {
		return nil, "", nil
	}
	// Bound the fetch in time AND size: a slow host must not stall the request, and
	// a malicious/misconfigured one must not stream gigabytes into memory (the
	// timeout alone is not a size bound). Both degrade to a generated avatar.
	dlCtx, cancel := context.WithTimeout(ctx, githubAvatarFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, "", nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", nil
	}
	// LimitReader(+1) detects (not silently truncates) an oversize body.
	data, err := io.ReadAll(io.LimitReader(resp.Body, icons.MaxBytes+1))
	if err != nil {
		return nil, "", nil
	}
	if len(data) > icons.MaxBytes {
		return nil, "", nil
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	return data, ct, nil
}

// githubAvatarURL shells out to git + gh to resolve the owner avatar URL.
func githubAvatarURL(
	ctx context.Context,
	repoPath string,
) string {
	//nolint:gosec // G204: fixed git subcommand; repoPath is a daemon-managed repo path, not shell-interpreted or attacker-controlled.
	raw, err := exec.CommandContext(ctx, binpath.Git(), "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	slug, err := githubSlugFromURL(strings.TrimSpace(string(raw)))
	if err != nil {
		return ""
	}
	// binpath.Resolve: the packaged .app daemon inherits launchd's minimal PATH,
	// which misses Homebrew's /opt/homebrew/bin where gh usually lives.
	//nolint:gosec // G204: gh invoked with fixed args; slug is parsed from the repo's own git remote URL and passed as a discrete argv entry, not shell-interpreted.
	out, err := exec.CommandContext(ctx, binpath.Resolve("gh"), "api", "repos/"+slug, "--jq", ".owner.avatar_url").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// githubSlugFromURL extracts "owner/repo" from a GitHub remote URL.
func githubSlugFromURL(
	rawURL string,
) (string, error) {
	rawURL = strings.TrimSuffix(strings.TrimSpace(rawURL), ".git")
	if strings.HasPrefix(rawURL, "git@") {
		parts := strings.SplitN(rawURL, ":", 2)
		if len(parts) == 2 {
			return parts[1], nil
		}
	}
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		path := rawURL[idx+3:]
		if slash := strings.Index(path, "/"); slash >= 0 {
			return path[slash+1:], nil
		}
	}
	return "", fmt.Errorf("unrecognised URL: %q", rawURL)
}

// Branches handles GET /v0/projects/:projectId/repos/:repoId/branches. Returns all remote branches
// annotated with isProtected and hasWorkspace fields.
//
// It refreshes the remote-tracking refs FIRST. `git branch -r` reads only what
// the clone already knows, and no other part of the daemon does a full fetch on
// its own (OriginSyncManager refreshes a single subscribed protected branch;
// Fetch is the user's Git-panel button) — so this list used to report the remote
// as it stood at clone time: a teammate's branch pushed since the repo was
// imported never appeared in the import picker, and a branch deleted on the
// remote was offered forever. The refresh is best-effort: an offline machine
// still gets the cached list rather than an error.
func (h *Handlers) Branches(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	ctx := c.Request.Context()
	if h.remote == nil {
		libs.WriteErr(c, http.StatusInternalServerError, "branch listing is not wired")
		return
	}
	if fErr := h.remote.FetchPrune(ctx, repo.Path); fErr != nil {
		slog.WarnContext(ctx, "branches: could not refresh origin; listing cached remote-tracking refs",
			"repo", repo.Name, "err", fErr)
	}
	// Through the git engine and its per-repo lock, never a bare shell-out.
	all, err := h.remote.Branches(ctx, repo.Path)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to list branches")
		return
	}
	rawBranches := originBranches(all)

	// A protection lookup that fails must not report every branch as
	// unprotected: the picker would offer to import a protected branch as an
	// ordinary one.
	protected := map[string]bool{}
	if h.provider != nil {
		list, pErr := h.provider.ProtectedBranches(ctx, repo.Path)
		if pErr != nil {
			libs.WriteErr(c, http.StatusBadGateway, "failed to read protected branches")
			return
		}
		for _, b := range list {
			protected[b] = true
		}
	}

	// Annotate with workspace existence. The default workspace is the imported
	// folder itself — an unmanaged checkout that merely happens to sit on some
	// branch. Crowbar does not own that branch, so it must NOT count as "already
	// imported": the user is free to import that same branch as a real managed
	// workspace. Skip IsDefault here.
	hasWS := map[string]bool{}
	if h.wsReader != nil {
		rows, lErr := h.wsReader.List(ctx)
		if lErr != nil {
			libs.WriteErr(c, http.StatusInternalServerError, "failed to list workspaces")
			return
		}
		for _, ws := range rows {
			if ws.RepoID == repo.ID && !ws.IsDefault && ws.Status != domain.WorkspaceStatusDeleted {
				hasWS[ws.Branch] = true
			}
		}
	}

	entries := make([]BranchEntry, 0, len(rawBranches))
	for _, b := range rawBranches {
		entries = append(entries, BranchEntry{
			Name:         b,
			IsProtected:  protected[b],
			HasWorkspace: hasWS[b],
		})
	}
	libs.WriteQueryOK(c, entries)
}

// PRLinkDTO is one head→base edge of the repo's open-PR graph, returned by
// GET …/repos/:repoId/pull-requests for the import dialog's parent hint.
type PRLinkDTO struct {
	Head   string `json:"head"`
	Base   string `json:"base"`
	Number int    `json:"number"`
	Status string `json:"status"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

// PullRequests handles GET /v0/projects/:projectId/repos/:repoId/pull-requests.
// Returns the open-PR head→base graph for the import dialog's parent hint. It is
// advisory only — the import endpoint re-resolves parenting authoritatively — and
// soft-fails to [] when the provider CLI is unavailable or unauthenticated.
func (h *Handlers) PullRequests(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	links := []PRLinkDTO{}
	if h.provider != nil {
		got, _ := h.provider.OpenPullRequests(c.Request.Context(), repo.Path)
		for _, l := range got {
			links = append(links, PRLinkDTO{
				Head:   l.Head,
				Base:   l.Base,
				Number: l.Number,
				Status: l.Status,
				URL:    l.URL,
				Title:  l.Title,
			})
		}
	}
	libs.WriteQueryOK(c, links)
}

// originBranches keeps the engine's origin/ remote-tracking branches, with the
// "origin/" prefix stripped. Only origin is kept: every other remote's branch
// would be offered as importable and then resolved against origin/<name>,
// which may be a different branch or none at all. origin/HEAD (short name
// "origin") has no prefix and falls away.
func originBranches(all []gitdomain.Branch) []string {
	var result []string
	seen := map[string]bool{}
	for _, b := range all {
		if !b.IsRemote {
			continue
		}
		name, ok := strings.CutPrefix(b.Name, "origin/")
		if !ok || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
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

// PutIconEmoji handles PUT /v0/projects/:projectId/repos/:repoId/icon/emoji.
// Body: {"emoji":"🦊"} — stores "emoji:🦊" in avatar_url.
func (h *Handlers) PutIconEmoji(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	var body struct {
		Emoji string `json:"emoji"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Emoji == "" {
		libs.WriteErr(c, http.StatusBadRequest, "emoji required")
		return
	}
	body.Emoji = strings.TrimSpace(body.Emoji)
	if !icons.IsSingleEmoji(body.Emoji) {
		libs.WriteErr(c, http.StatusBadRequest, "emoji must be a single character")
		return
	}
	// Emoji takes precedence over an on-disk image: clear the icon flag and
	// best-effort remove any previously stored icon file.
	if iconPath, ok := h.iconPath(c); ok {
		_ = os.Remove(iconPath)
	}
	repo.AvatarEmoji = body.Emoji
	repo.AvatarHasIcon = false
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Deliver the updated avatar to every client on the repos WS stream — the
	// store Save alone does not fan out.
	h.broadcast(dto.RepoDTOFrom(*repo, h.placementOf(c.Request.Context(), repo.ID)))
	c.Status(http.StatusNoContent)
}

// DeleteIcon handles DELETE /v0/projects/:projectId/repos/:repoId/icon.
// Removes the on-disk icon file and clears both the icon flag and the emoji,
// resetting the repo to its generated label/color avatar.
func (h *Handlers) DeleteIcon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	if iconPath, ok := h.iconPath(c); ok {
		_ = os.Remove(iconPath)
	}
	repo.AvatarHasIcon = false
	repo.AvatarEmoji = ""
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Deliver the updated avatar to every client on the repos WS stream — the
	// store Save alone does not fan out.
	h.broadcast(dto.RepoDTOFrom(*repo, h.placementOf(c.Request.Context(), repo.ID)))
	c.Status(http.StatusNoContent)
}

// githubAvatarFetchTimeout bounds the outbound GitHub owner-avatar download so a
// slow host never stalls the icon-refresh path.
const githubAvatarFetchTimeout = 10 * time.Second

// PutIcon handles PUT /v0/projects/:projectId/repos/:repoId/icon. It accepts the
// icon two ways: a multipart/form-data "icon" field (web browsers), or a JSON
// body {"path": "<absolute path>"} the daemon reads from disk itself. The latter
// is the desktop path: the WKWebView crowbar:// scheme cannot carry a
// multipart/binary request body, so the native file dialog yields a path and the
// daemon reads it — the same way repo import reads a user-selected path.
// Accepts image/png, image/jpeg, image/webp; max 5 MB.
func (h *Handlers) PutIcon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	data, ok := icons.ReadUpload(c)
	if !ok {
		return
	}
	if !icons.Validate(c, data) {
		return
	}
	if err := h.storeIconBytes(c, data); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}

	repo.AvatarHasIcon = true
	repo.AvatarEmoji = ""
	// New bytes behind the stable icon URL: bump the version so the DTO's
	// ?v= param changes and clients refetch the image.
	repo.AvatarVersion++
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 204, consistent with the other icon mutations: the FE apiFetch throws on
	// any non-enveloped 200 body, and the updated avatar is delivered on the
	// repos WS stream by the broadcast below, not in this response.
	h.broadcast(dto.RepoDTOFrom(*repo, h.placementOf(c.Request.Context(), repo.ID)))
	c.Status(http.StatusNoContent)
}

// storeIconBytes writes raw icon bytes to this repo's entity-scoped icon path.
func (h *Handlers) storeIconBytes(
	c *gin.Context,
	data []byte,
) error {
	iconPath, ok := h.iconPath(c)
	if !ok {
		return fmt.Errorf("could not resolve icon path")
	}
	return icons.Store(iconPath, data)
}

// PutIconGithub handles PUT /v0/projects/:projectId/repos/:repoId/icon/github.
// Downloads the repo owner's GitHub avatar bytes and stores them at the
// entity-scoped icon path, setting AvatarHasIcon.
func (h *Handlers) PutIconGithub(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	if repo.Path == "" {
		libs.WriteErr(c, http.StatusUnprocessableEntity, "repo has no local path")
		return
	}
	data, _, err := h.fetchAvatar(c.Request.Context(), repo.Path)
	if err != nil || len(data) == 0 {
		libs.WriteErr(c, http.StatusUnprocessableEntity, "could not fetch GitHub avatar")
		return
	}
	if err := h.storeIconBytes(c, data); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	repo.AvatarHasIcon = true
	repo.AvatarEmoji = ""
	// New bytes behind the stable icon URL: bump the version so the DTO's
	// ?v= param changes and clients refetch the image.
	repo.AvatarVersion++
	if err := h.store.Save(c.Request.Context(), *repo); err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Deliver the updated avatar to every client on the repos WS stream — the
	// store Save alone does not fan out.
	h.broadcast(dto.RepoDTOFrom(*repo, h.placementOf(c.Request.Context(), repo.ID)))
	c.Status(http.StatusNoContent)
}

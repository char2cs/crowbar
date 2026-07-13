// Package handlers holds the gin handlers backing the repos endpoint.
package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rivo/uniseg"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var avatarColors = []string{
	"bg-indigo-700", "bg-emerald-700", "bg-orange-700", "bg-sky-700",
	"bg-rose-700", "bg-violet-700", "bg-teal-700", "bg-amber-700",
}

// repoAvatar derives a 1-2 char label and deterministic Tailwind color from a repo name.
func repoAvatar(name string) (label, color string) {
	words := strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, name))
	switch len(words) {
	case 0:
		label = "R"
	case 1:
		r, _ := utf8.DecodeRuneInString(words[0])
		label = strings.ToUpper(string(r))
	default:
		r0, _ := utf8.DecodeRuneInString(words[0])
		r1, _ := utf8.DecodeRuneInString(words[1])
		label = strings.ToUpper(string(r0) + string(r1))
	}
	hash := 0
	for _, c := range name {
		hash = (hash*31 + int(c)) & 0xFFFFFF
	}
	color = avatarColors[hash%len(avatarColors)]
	return label, color
}

// gitRemoteURL returns the origin remote URL for the repo at path, or "".
func gitRemoteURL(path string) string {
	//nolint:gosec // G204: fixed git subcommand; path is a daemon-managed repo path, not shell-interpreted or attacker-controlled.
	out, err := exec.Command("git", "-C", path, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

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

// BranchProviderEngine is the provider surface the Branches handler needs.
type BranchProviderEngine interface {
	ProtectedBranches(ctx context.Context, repoPath string) ([]string, error)
}

// WorkspaceReader is the workspace surface the Branches handler needs.
type WorkspaceReader interface {
	List(ctx context.Context) ([]domain.Workspace, error)
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
		repoPath string,
	) (domain.Repository, error)
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
	importer    RepoImporter
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
		crowbarHome: defaultCrowbarHome,
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
		crowbarHome: defaultCrowbarHome,
		fetchAvatar: fetchGithubAvatarBytes,
		broadcast:   broadcast,
		stat:        os.Stat,
	}
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
	repos = filterByProject(repos, c.Query("projectId"))
	libs.WriteQueryOK(c, dto.RepoDTOList(repos))
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
	libs.WriteQueryOK(c, dto.RepoDTOFrom(*repo))
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
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	if _, err := h.stat(body.Path); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "path does not exist")
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(c.Request.Context(), func(ctx context.Context) {
		repo, ok := h.persistRepo(ctx, body)
		if !ok {
			return
		}
		h.broadcast(dto.RepoDTOFrom(repo))
	})
}

// persistRepo runs the background create work. When a full RepoImporter is
// wired it runs the complete import (default-branch workspace adoption +
// protected-branch stubs + GitHub avatar), which also broadcasts the adopted
// workspaces via the workspace repo callback. Without an importer it falls back
// to the bare buildRepo+Save path. ok is false when the work failed and no
// RepoDTO should be broadcast (no per-repo LastError sink).
func (h *Handlers) persistRepo(
	ctx context.Context,
	body createRequest,
) (domain.Repository, bool) {
	if h.importer != nil {
		repo, err := h.importer.ImportRepo(ctx, body.ProjectID, body.Path)
		if err != nil {
			return domain.Repository{}, false
		}
		return repo, true
	}
	repo := buildRepo(body)
	if err := h.store.Save(ctx, repo); err != nil {
		return domain.Repository{}, false
	}
	return repo, true
}

// buildRepo derives the persisted Repository from the validated create request:
// a generated id when absent, the git-derived default branch and remote URL when
// a local path is present, and the generated label/color avatar.
func buildRepo(
	body createRequest,
) domain.Repository {
	defaultBranch := body.DefaultBranch
	if defaultBranch == "" && body.Path != "" {
		defaultBranch = gitDefaultBranch(body.Path)
	}
	id := body.ID
	if id == "" {
		id = uuid.NewString()
	}
	remoteURL := ""
	if body.Path != "" {
		remoteURL = gitRemoteURL(body.Path)
	}
	label, color := repoAvatar(body.Name)
	return domain.Repository{
		ID:            id,
		ProjectID:     body.ProjectID,
		Name:          body.Name,
		Path:          body.Path,
		DefaultBranch: defaultBranch,
		RemoteURL:     remoteURL,
		AvatarLabel:   label,
		AvatarColor:   color,
	}
}

// DeleteRepo handles DELETE /v0/projects/:projectId/repos/:repoId. It validates
// the repo exists synchronously (4xx if not), then returns 202 and runs the
// removal in the background: the GORM record is deleted and the entity-scoped
// repo directory (worktrees, icon, storages) is torn down on disk. A
// deleted-status RepoDTO tombstone is broadcast on the Repos WebSocket stream
// so the client cache drops the entity (00 §6). The user's real repository
// directory (repo.Path) is never touched — only the ~/.crowbar entity dir.
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
	home, _ := h.crowbarHome()
	libs.WriteAccepted(c)
	h.runAsync(c.Request.Context(), func(ctx context.Context) {
		if err := h.store.Delete(ctx, repoID); err != nil {
			return
		}
		if home != "" {
			_ = os.RemoveAll(repoDir(home, projectID, repoID))
		}
		h.broadcast(dto.RepoDTO{ID: repoID, ProjectID: projectID, Status: "deleted"})
	})
}

// repoDir mirrors worktreepath.RepoDir without importing the usecase-internal
// package (forbidden from the api layer):
// <crowbarHome>/projects/<projectID>/<repoID>.
func repoDir(
	crowbarHome string,
	projectID string,
	repoID string,
) string {
	return filepath.Join(crowbarHome, "projects", projectID, repoID)
}

// gitDefaultBranch reads the current branch from a git repository at path.
// Returns "" if path is not a git repo or the command fails.
func gitDefaultBranch(
	path string,
) string {
	//nolint:gosec // G204: fixed git subcommand; path is a daemon-managed repo path, not shell-interpreted or attacker-controlled.
	out, err := exec.Command("git", "-C", path, "symbolic-ref", "HEAD", "--short").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	// Stat-reject and cap the read: icons are stored by this daemon, but a
	// corrupted or replaced file should not cause an unbounded heap allocation.
	info, err := os.Stat(iconPath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if info.Size() > maxIconBytes {
		c.Status(http.StatusNotFound)
		return
	}
	//nolint:gosec // G304: iconPath comes from the daemon's entity-scoped icon store (h.iconPath), already stat-checked and size-capped above, not user-supplied.
	f, err := os.Open(iconPath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxIconBytes+1))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	// no-cache: revalidate on every use. The bytes change in place behind this
	// URL (uploads overwrite the same file); the ?v= param on the DTO URL is
	// the primary cache-buster, this header is the belt-and-braces layer.
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, iconContentType(data), data)
}

// iconContentType picks the Content-Type for a stored icon. http.DetectContentType
// has no SVG signature — it sniffs SVG as text/* — and browsers refuse to render
// an <img> whose SVG is served as text/*. Some GitHub owner avatars are SVG (e.g.
// org avatars), so detect SVG explicitly and serve image/svg+xml; otherwise the
// fetched icon silently degrades to the generated label placeholder. Real raster
// images keep their sniffed image/* type.
func iconContentType(data []byte) string {
	ct := http.DetectContentType(data)
	if strings.HasPrefix(ct, "image/") {
		return ct
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	if strings.Contains(string(head), "<svg") {
		return "image/svg+xml"
	}
	return ct
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

// defaultCrowbarHome returns the root for all crowbar-managed state: the
// CROWBAR_HOME env override when set (dev instances point it inside the
// workspace being developed), otherwise ~/.crowbar.
func defaultCrowbarHome() (string, error) {
	if override := os.Getenv(metadata.HomeEnvVar); override != "" {
		return override, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("crowbar home: %w", err)
	}
	return filepath.Join(h, ".crowbar"), nil
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIconBytes+1))
	if err != nil {
		return nil, "", nil
	}
	if len(data) > maxIconBytes {
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
	raw, err := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", "origin").Output()
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
func (h *Handlers) Branches(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}

	// List remote branches via git branch -r
	//nolint:gosec // G204: fixed git subcommand; repo.Path is a daemon-managed repo path, not shell-interpreted or attacker-controlled.
	cmd := exec.CommandContext(c.Request.Context(), "git", "-C", repo.Path, "branch", "-r", "--format=%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to list branches")
		return
	}
	rawBranches := parseRemoteBranches(string(out))

	// Annotate with protected status
	protected := map[string]bool{}
	if h.provider != nil {
		list, _ := h.provider.ProtectedBranches(c.Request.Context(), repo.Path)
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
		all, _ := h.wsReader.List(c.Request.Context())
		for _, ws := range all {
			if ws.RepoID == repo.ID && !ws.IsDefault {
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

// parseRemoteBranches strips the "origin/" prefix from git branch -r output
// and skips HEAD pointer lines.
func parseRemoteBranches(out string) []string {
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "->") {
			continue
		}
		if idx := strings.Index(line, "/"); idx >= 0 {
			line = line[idx+1:]
		}
		result = append(result, line)
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
	if !isSingleEmoji(body.Emoji) {
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
	h.broadcast(dto.RepoDTOFrom(*repo))
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
	h.broadcast(dto.RepoDTOFrom(*repo))
	c.Status(http.StatusNoContent)
}

// maxIconBytes caps the stored icon at 2 MiB. Any file larger than this is
// rejected before or immediately after opening, so the daemon never reads an
// unbounded amount of data from a client-supplied path.
const maxIconBytes = 2 << 20

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
	data, _, ok := h.readIconUpload(c)
	if !ok {
		return
	}
	if len(data) > maxIconBytes {
		libs.WriteErr(c, http.StatusBadRequest, "icon must be under 2 MB")
		return
	}
	// Always validate by content sniffing (not by trusting the extension or the
	// caller-supplied Content-Type) so a non-image file cannot be stored by
	// supplying a .png filename or a JSON path to an arbitrary host file.
	sniffed := http.DetectContentType(data)
	if !strings.HasPrefix(sniffed, "image/") {
		libs.WriteErr(c, http.StatusBadRequest, "icon must be an image file")
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
	h.broadcast(dto.RepoDTOFrom(*repo))
	c.Status(http.StatusNoContent)
}

// readIconUpload extracts the icon bytes and content-type from the request,
// dispatching on Content-Type: a JSON {"path"} body (desktop, daemon reads the
// file) or a multipart "icon" field (web). On any error it writes the response
// and returns ok=false.
func (h *Handlers) readIconUpload(c *gin.Context) (data []byte, contentType string, ok bool) {
	if strings.HasPrefix(c.ContentType(), "application/json") {
		return readIconFromPath(c)
	}
	return readIconFromMultipart(c)
}

// readIconFromPath reads the icon from an absolute path supplied as JSON.
//
// Residual trust assumption: the path is an absolute host path supplied by the
// desktop client (native file-picker dialog). The daemon and the WKWebView run
// on the same host, so this is equivalent to the repo-import path trust model:
// the path is user-chosen, not attacker-controlled from the network. This path
// should eventually be replaced by a byte-upload (multipart) so the daemon never
// reads arbitrary host paths at client direction.
//
// Hardening applied:
//   - Stat-reject: file must exist and be ≤ maxIconBytes before any read.
//   - LimitReader: read at most maxIconBytes+1 so an oversize file is detected.
//   - Content sniffing: content-type is derived from the first 512 bytes, not
//     from the file extension, so /etc/passwd styled as photo.png is rejected.
func readIconFromPath(c *gin.Context) ([]byte, string, bool) {
	var body struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "icon path required")
		return nil, "", false
	}
	// Stat-reject before opening: avoids an unbounded read on a huge file.
	info, err := os.Stat(body.Path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "could not read icon file")
		return nil, "", false
	}
	if info.Size() > maxIconBytes {
		libs.WriteErr(c, http.StatusBadRequest, "icon must be under 2 MB")
		return nil, "", false
	}
	f, err := os.Open(body.Path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "could not read icon file")
		return nil, "", false
	}
	defer func() { _ = f.Close() }()
	// LimitReader caps the actual read even if the file grows between Stat and Open.
	data, err := io.ReadAll(io.LimitReader(f, maxIconBytes+1))
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "could not read icon file")
		return nil, "", false
	}
	if int64(len(data)) > maxIconBytes {
		libs.WriteErr(c, http.StatusBadRequest, "icon must be under 2 MB")
		return nil, "", false
	}
	// Derive content-type by sniffing bytes, not by trusting the file extension.
	ct := http.DetectContentType(data)
	return data, ct, true
}

// readIconFromMultipart reads the icon from a multipart "icon" form field.
// The read is capped at maxIconBytes+1 so an oversize upload is detected without
// buffering the entire body. Content-type is derived from content sniffing.
func readIconFromMultipart(c *gin.Context) ([]byte, string, bool) {
	file, _, err := c.Request.FormFile("icon")
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "icon field required")
		return nil, "", false
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxIconBytes+1))
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "read error")
		return nil, "", false
	}
	// Content-type from content sniffing (not from the caller-supplied MIME header
	// or filename extension) so the caller cannot sneak a non-image through.
	ct := http.DetectContentType(data)
	return data, ct, true
}

// storeIconBytes writes raw icon bytes to the entity-scoped icon path,
// creating the parent dir. The single icon file is content-type-agnostic
// (sniffed on read), so there is no extension to manage.
func (h *Handlers) storeIconBytes(
	c *gin.Context,
	data []byte,
) error {
	iconPath, ok := h.iconPath(c)
	if !ok {
		return fmt.Errorf("could not resolve icon path")
	}
	//nolint:gosec // G301: 0o755 is the intended perm for the daemon's own icon directory; kept as-is to preserve existing behavior.
	if err := os.MkdirAll(filepath.Dir(iconPath), 0o755); err != nil {
		return fmt.Errorf("could not create icon directory: %w", err)
	}
	//nolint:gosec // G306: icon bytes are non-secret assets served over HTTP; 0o644 is the intended readable perm, kept as-is to preserve behavior.
	if err := os.WriteFile(iconPath, data, 0o644); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	return nil
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
	h.broadcast(dto.RepoDTOFrom(*repo))
	c.Status(http.StatusNoContent)
}

// isSingleEmoji returns true when s is a non-empty string containing exactly
// one user-perceived character (grapheme cluster) that is not a plain ASCII
// letter. Grapheme clusters — not code points — are the unit that matters:
// most real emoji are multi-codepoint sequences (❤️ carries a variation
// selector, 👨‍💻 is a ZWJ sequence, 🇦🇷 is a two-codepoint flag, 👍🏽 carries a
// skin-tone modifier) and must all be accepted as "a single emoji".
func isSingleEmoji(s string) bool {
	if s == "" {
		return false
	}
	g := uniseg.NewGraphemes(s)
	if !g.Next() {
		return false
	}
	if g.Next() {
		return false // more than one user-perceived character
	}
	r, _ := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return false
	}
	return !unicode.IsLetter(r) || r > 127
}

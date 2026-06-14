// Package handlers holds the gin handlers backing the repos endpoint.
package handlers

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the full surface the repos handlers need over the repository GORM
// table: list every repo, fetch one by id, and persist a new one.
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
}

// Handlers serves the /v0/repos routes from the repository GORM store.
type Handlers struct {
	store Store
}

// New builds the repos Handlers from the repository GORM store.
func New(
	store Store,
) *Handlers {
	return &Handlers{store: store}
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

// Detail handles GET /v0/repos/:id, returning a single RepoDTO. The workspace
// tree is not yet composed by any usecase, so detail carries the repo fields
// only.
func (h *Handlers) Detail(
	c *gin.Context,
) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("id"))
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

// Create handles POST /v0/repos, persisting a new repository record. The
// defaultBranch field is optional: when omitted the handler derives it from the
// local git repository at path via symbolic-ref HEAD.
func (h *Handlers) Create(
	c *gin.Context,
) {
	var body struct {
		ID            string `json:"id"`
		ProjectID     string `json:"projectId"`
		Name          string `json:"name"`
		Path          string `json:"path"`
		DefaultBranch string `json:"defaultBranch"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	defaultBranch := body.DefaultBranch
	if defaultBranch == "" && body.Path != "" {
		defaultBranch = gitDefaultBranch(body.Path)
	}
	repo := domain.Repository{
		ID:            body.ID,
		ProjectID:     body.ProjectID,
		Name:          body.Name,
		Path:          body.Path,
		DefaultBranch: defaultBranch,
	}
	if err := h.store.Save(c.Request.Context(), repo); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, repo.ID)
}

// gitDefaultBranch reads the current branch from a git repository at path.
// Returns "" if path is not a git repo or the command fails.
func gitDefaultBranch(
	path string,
) string {
	out, err := exec.Command("git", "-C", path, "symbolic-ref", "HEAD", "--short").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Icon handles GET /v0/repos/:id/icon. If AvatarURL is an HTTPS URL it
// redirects. If it is a local filesystem path it reads and serves the file.
func (h *Handlers) Icon(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("id"))
	if err != nil || repo == nil || repo.AvatarURL == "" {
		c.Status(http.StatusNotFound)
		return
	}
	if strings.HasPrefix(repo.AvatarURL, "http") {
		c.Redirect(http.StatusTemporaryRedirect, repo.AvatarURL)
		return
	}
	data, err := os.ReadFile(repo.AvatarURL)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	contentTypes := map[string]string{
		".svg":  "image/svg+xml",
		".png":  "image/png",
		".ico":  "image/x-icon",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
	}
	ct := contentTypes[strings.ToLower(filepath.Ext(repo.AvatarURL))]
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.Data(http.StatusOK, ct, data)
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

package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Branches handles GET /v0/projects/:projectId/repos/:repoId/branches, listing origin's branches
// with isProtected and hasWorkspace. It fetches with prune first because nothing else does a full
// fetch; the refresh is best-effort so an offline machine still gets the cached list.
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
	all, err := h.remote.Branches(ctx, repo.Path)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to list branches")
		return
	}
	rawBranches := originBranches(all)

	protected, err := h.protectedSet(ctx, repo.Path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadGateway, "failed to read protected branches")
		return
	}
	hasWS, err := h.managedBranches(ctx, repo.ID)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to list workspaces")
		return
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

// protectedSet is the repo's protected branches. A lookup that fails is an
// error, not "none protected": the picker would offer a protected branch as an
// ordinary one.
func (h *Handlers) protectedSet(ctx context.Context, repoPath string) (map[string]bool, error) {
	protected := map[string]bool{}
	if h.provider == nil {
		return protected, nil
	}
	list, err := h.provider.ProtectedBranches(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("protected branches: %w", err)
	}
	for _, b := range list {
		protected[b] = true
	}
	return protected, nil
}

// managedBranches is the branches the repo has a live managed workspace on. The
// default workspace is the imported folder itself, an unmanaged checkout of
// whatever branch it sits on, so it does not make that branch "imported".
func (h *Handlers) managedBranches(ctx context.Context, repoID string) (map[string]bool, error) {
	hasWS := map[string]bool{}
	if h.wsReader == nil {
		return hasWS, nil
	}
	rows, err := h.wsReader.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	for _, ws := range rows {
		if ws.RepoID == repoID && !ws.IsDefault && ws.Status != domain.WorkspaceStatusDeleted {
			hasWS[ws.Branch] = true
		}
	}
	return hasWS, nil
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

// PullRequests handles GET /v0/projects/:projectId/repos/:repoId/pull-requests: the open-PR graph
// as an advisory parent hint (import re-resolves parenting), soft-failing to [] without a provider CLI.
func (h *Handlers) PullRequests(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	links := []PRLinkDTO{}
	if h.provider != nil {
		got, err := h.provider.OpenPullRequests(c.Request.Context(), repo.Path)
		if err != nil { // an advisory hint: no provider CLI or no auth is the common case
			slog.DebugContext(c.Request.Context(), "repos: open pull requests", "repo_id", repo.ID, "err", err)
		}
		for _, l := range got {
			links = append(links, PRLinkDTO{
				Head: l.Head, Base: l.Base, Number: l.Number,
				Status: l.Status, URL: l.URL, Title: l.Title,
			})
		}
	}
	libs.WriteQueryOK(c, links)
}

// originBranches keeps origin's remote-tracking branches with the prefix stripped. Other remotes
// are dropped because import resolves a name against origin/<name>; origin/HEAD ("origin")
// has no prefix and falls away.
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

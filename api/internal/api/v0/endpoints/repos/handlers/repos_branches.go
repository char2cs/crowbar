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

type PRLinkDTO struct {
	Head   string `json:"head"`
	Base   string `json:"base"`
	Number int    `json:"number"`
	Status string `json:"status"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

func (h *Handlers) PullRequests(c *gin.Context) {
	repo, err := h.store.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil || repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	links := []PRLinkDTO{}
	if h.provider != nil {
		got, err := h.provider.OpenPullRequests(c.Request.Context(), repo.Path)
		if err != nil {
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

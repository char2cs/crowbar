package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
)

// importRequest is the POST …/workspaces/import body: the branches to import as
// managed workspaces. Each is PR-parented up to a protected/default root, with
// missing ancestors created (see workspace.CreateFromImport).
type importRequest struct {
	Branches []string `json:"branches"`
}

// Import handles POST /v0/projects/:projectId/repos/:repoId/workspaces/import.
// It validates synchronously (repo present, branches non-empty), returns 202,
// and resolves + creates the tree in the background. Each created workspace
// arrives on the per-repo workspace WebSocket stream, exactly like a single
// create; a batch that fails before producing any workspace is best-effort
// logged (no entity to hang LastError on).
func (h *Handlers) Import(c *gin.Context) {
	var body importRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Branches) == 0 {
		libs.WriteErr(c, http.StatusBadRequest, "branches is required")
		return
	}
	repo, err := h.repos.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	if repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	// The chain walk TERMINATES at the default branch, so importing it creates
	// nothing — and a 202 for work defined to do nothing is a silent hang: the
	// caller's optimistic row is cleared only by a workspace that never arrives.
	// Refuse it here, on the request path, where the caller can surface it.
	for _, b := range body.Branches {
		if b == repo.DefaultBranch {
			libs.WriteErr(c, http.StatusConflict, fmt.Sprintf(
				"%s is the repository's default branch — it is already the repo's home workspace", b,
			))
			return
		}
	}
	// Same reasoning for a branch that is not on the remote at all — a stale
	// picker offering a branch deleted since the list was fetched, or a
	// hand-crafted request. Importing means "give me origin's branch", so
	// without one there is nothing to import: the batch would fail deep in the
	// background, where the only channel it has is runAsync's blank workspace id
	// (a no-op in broadcastLastError), and the caller's row would spin forever.
	if missing := h.branchesMissingFromRemote(c.Request.Context(), repo.Path, body.Branches); len(missing) > 0 {
		libs.WriteErr(c, http.StatusBadRequest, fmt.Sprintf(
			"not found on the remote: %s", strings.Join(missing, ", "),
		))
		return
	}
	in := workspace.ImportInput{
		RepoID:        repo.ID,
		ProjectID:     repo.ProjectID,
		RepoPath:      repo.Path,
		RemoteURL:     repo.RemoteURL,
		DefaultBranch: repo.DefaultBranch,
		Branches:      body.Branches,
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		"",
		func(ctx context.Context) error {
			if importErr := h.hierarchy.CreateFromImport(ctx, in); importErr != nil {
				slog.WarnContext(ctx, "workspace import failed",
					"repo", in.RepoID, "branches", in.Branches, "err", importErr)
				return importErr
			}
			return nil
		},
	)
}

// branchesMissingFromRemote refreshes the remote-tracking refs and returns the
// requested branches origin does not have, so Import can refuse them
// synchronously.
//
// The refresh runs HERE rather than trusting the picker's own GET …/branches:
// the check has to hold for any caller, and the branch may have been deleted on
// the remote between listing and importing. It is the same fetch the import
// would perform per branch anyway, hoisted ahead of the 202 so its result can
// still be an HTTP error. Refusal is then decided on the LOCAL refs it just
// wrote, never a second live `ls-remote` — a hiccup in that query must never
// veto a branch the refs confirm (the bug that once fabricated a fresh fork off
// the default branch).
//
// Every uncertainty resolves to "not missing": a failed fetch (offline — absence
// is unprovable), an unreadable ref, or no git surface wired at all (tests). The
// usecase's per-branch placeholder rows stay the backstop for those.
func (h *Handlers) branchesMissingFromRemote(
	ctx context.Context,
	repoPath string,
	branches []string,
) []string {
	if h.remote == nil || repoPath == "" {
		return nil
	}
	if err := h.remote.FetchPrune(ctx, repoPath); err != nil {
		slog.WarnContext(ctx, "import: could not refresh origin; importing without the branch-exists check",
			"repo", repoPath, "err", err)
		return nil
	}
	var missing []string
	for _, b := range branches {
		exists, err := h.remote.RemoteTrackingBranchExists(ctx, repoPath, b)
		if err != nil {
			slog.WarnContext(ctx, "import: could not check remote-tracking ref; not refusing the branch",
				"branch", b, "err", err)
			continue
		}
		if !exists {
			missing = append(missing, b)
		}
	}
	return missing
}

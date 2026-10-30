package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// importRequest is the POST …/chats/import-batch body: each branch to import,
// the chat id the caller draws it under, and whether it is created locked.
type importRequest struct {
	Branches []importBranchRequest `json:"branches"`
}

type importBranchRequest struct {
	Name   string `json:"name"`
	ChatID string `json:"chatId"`
	Lock   bool   `json:"lock"`
}

// Import handles POST …/repos/:repoId/chats/import-batch. What it can check up
// front is a 4xx; then it answers 202 with one running item per branch, which
// the work settles branch by branch.
func (h *Handlers) Import(c *gin.Context) {
	var body importRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := checkImportBranches(body.Branches); err != nil {
		libs.Fail(c, err)
		return
	}
	repo, err := h.repos.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil {
		libs.Fail(c, err)
		return
	}
	if repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	names := branchNames(body.Branches)
	// The chain walk TERMINATES at the default branch, so importing it would
	// create nothing; refused here, where the caller can surface it.
	if slices.Contains(names, repo.DefaultBranch) {
		libs.WriteErr(c, http.StatusConflict, fmt.Sprintf(
			"%s is the repository's default branch — it is already the repo's home workspace", repo.DefaultBranch,
		))
		return
	}
	// A branch not on the remote (a stale picker, a hand-crafted request) has
	// nothing to import; the refusal belongs on the request.
	if missing := h.branchesMissingFromRemote(c.Request.Context(), repo.Path, names); len(missing) > 0 {
		libs.WriteErr(c, http.StatusBadRequest, fmt.Sprintf(
			"not found on the remote: %s", strings.Join(missing, ", "),
		))
		return
	}
	h.startImport(c, repo, body)
}

func (h *Handlers) startImport(c *gin.Context, repo *domain.Repository, body importRequest) {
	in := workspace.ImportInput{
		RepoID:        repo.ID,
		ProjectID:     repo.ProjectID,
		RepoPath:      repo.Path,
		RemoteURL:     repo.RemoteURL,
		DefaultBranch: repo.DefaultBranch,
	}
	items := make([]domain.OperationItem, 0, len(body.Branches))
	for _, b := range body.Branches {
		in.Branches = append(in.Branches, workspace.ImportBranch(b))
		items = append(items, domain.OperationItem{
			Ref: domain.EntityRef{Kind: domain.EntityChat, ID: b.ChatID}, Label: b.Name, State: domain.OpRunning,
		})
	}
	op, err := h.ops.Start(c.Request.Context(), operation.Spec{
		ID:        libs.IdempotencyKey(c),
		Kind:      domain.OpBranchImport,
		Subject:   domain.EntityRef{Kind: domain.EntityRepo, ID: repo.ID},
		ProjectID: repo.ProjectID,
		Label:     importLabel(branchNames(body.Branches)),
		Items:     items,
		// Only for a new key: a retry after the first import made these chats
		// gets the first operation back, not a refusal.
		Validate: func(ctx context.Context) error { return h.chatIDsUnused(ctx, body.Branches) },
	}, func(ctx context.Context, p operation.Progress) error {
		return h.hierarchy.CreateFromImport(ctx, in, importProgress{p})
	})
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteOperation(c, op)
}

// checkImportBranches refuses an empty batch, a nameless or repeated branch, or
// a chat id that is not a UUID or is given twice.
func checkImportBranches(branches []importBranchRequest) error {
	if len(branches) == 0 {
		return &problem.Error{Code: problem.InvalidArgument, Message: "branches is required"}
	}
	seen, named := map[string]bool{}, map[string]bool{}
	for _, b := range branches {
		if strings.TrimSpace(b.Name) == "" {
			return &problem.Error{Code: problem.InvalidArgument, Message: "every branch needs a name"}
		}
		if named[b.Name] {
			return &problem.Error{Code: problem.InvalidArgument, Message: b.Name + " is given twice"}
		}
		named[b.Name] = true
		if _, err := uuid.Parse(b.ChatID); err != nil {
			return &problem.Error{Code: problem.InvalidArgument, Message: fmt.Sprintf("chatId %q of %s is not a UUID", b.ChatID, b.Name)}
		}
		if seen[b.ChatID] {
			return &problem.Error{Code: problem.InvalidArgument, Message: "chatId " + b.ChatID + " is given twice"}
		}
		seen[b.ChatID] = true
	}
	return nil
}

// chatIDsUnused refuses a chat id that already names a chat.
func (h *Handlers) chatIDsUnused(ctx context.Context, branches []importBranchRequest) error {
	for _, b := range branches {
		_, err := h.chats.GetChat(ctx, b.ChatID)
		if err == nil {
			return &problem.Error{Code: problem.IDConflict, Message: fmt.Sprintf("chat %s already exists", b.ChatID)}
		}
		if problem.CodeOf(err) != problem.NotFound {
			return fmt.Errorf("import: check chat %s: %w", b.ChatID, err)
		}
	}
	return nil
}

// importProgress settles each branch's item from its outcome; an ancestor the
// import added arrives as an item of its own.
type importProgress struct {
	p operation.Progress
}

func (ip importProgress) Branch(o workspace.ImportOutcome) {
	item := domain.OperationItem{
		Ref:   domain.EntityRef{Kind: domain.EntityChat, ID: o.ChatID},
		Label: o.Branch,
		State: domain.OpSucceeded,
	}
	if o.Err != nil {
		failure := problem.Of(o.Err)
		item.State, item.Error = domain.OpFailed, &failure
	}
	ip.p.Item(item)
}

func branchNames(branches []importBranchRequest) []string {
	names := make([]string, 0, len(branches))
	for _, b := range branches {
		names = append(names, b.Name)
	}
	return names
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

func importLabel(branches []string) string {
	if len(branches) == 1 {
		return "Import " + branches[0]
	}
	return fmt.Sprintf("Import %d branches", len(branches))
}

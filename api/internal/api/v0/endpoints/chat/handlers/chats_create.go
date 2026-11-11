package handlers

import (
	"net/http"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// createRequest is the POST .../repos/:repoId/chats body. See Create.
type createRequest struct {
	// ID is the chat id the caller already draws the row under (a UUID);
	// omitted, the daemon mints one.
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	ParentID    string `json:"parentId"`
	WorkspaceID string `json:"workspaceId"`
	OwnWorktree bool   `json:"ownWorktree"`
	// Branch names the fresh branch an ownWorktree create cuts; blank gets a generated name. Unlike
	// Import.Branch, it names a branch that does not exist yet.
	Branch string               `json:"branch"`
	Import *createImportRequest `json:"import"`
	// Surface is the view the new chat is born on; omitted means the provider's
	// default. "terminal" births it on the provider's own CLI, whose PTY is the
	// conversation; a provider that cannot start there falls back at spawn.
	Surface string `json:"surface"`
}

// createImportRequest's presence selects WorktreeImport, so the three modes (none, ownWorktree,
// import) are explicit on the wire. Remote is a pointer because omitted means "the repo's remote"
// while an explicit "" means a purely local branch.
type createImportRequest struct {
	Branch string  `json:"branch"`
	Remote *string `json:"remote"`
}

// Create handles POST .../repos/:repoId/chats: places a new chat and starts its provider's CLI. The
// chat attaches to the home mount's :wsId, else the body's workspaceId, else no workspace; with none
// named, ownWorktree forks a fresh worktree and import adopts an existing branch.
func (h *Handlers) Create(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	var body createRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	wsID := ctx.Param("wsId")
	if wsID == "" {
		wsID = body.WorkspaceID
	}
	worktree, ok := h.worktreeSpec(ctx, body, wsID)
	if !ok {
		return
	}
	if !knownSurface(body.Surface) {
		libs.WriteErr(ctx, http.StatusBadRequest, "unknown surface: "+body.Surface)
		return
	}
	if _, err := uuid.Parse(body.ID); body.ID != "" && err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "id "+body.ID+" is not a UUID")
		return
	}

	// Through the tree usecase: placement under parentId must be written before the CLI starts,
	// because a thread learns it is one at spawn.
	created, err := h.folders.CreateChat(rctx, agentusecase.CreateChatInput{
		ID: body.ID, WorkspaceID: wsID, ProviderID: body.Provider, ParentID: body.ParentID,
		Worktree: worktree, Surface: body.Surface,
	})
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	chatID := created.ChatID
	libs.WriteQueryWithStatus(ctx, http.StatusCreated, createdData{ID: chatID, WorkspaceID: created.WorkspaceID})
}

// createdData answers a create: the chat, and the workspace a fork or an
// import minted for it.
type createdData struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

// knownSurface is a spelling check on the body's surface ("" means the provider's default); whether
// the provider can launch there is the descriptor's answer, resolved at spawn.
func knownSurface(surface string) bool {
	switch surface {
	case "", agents.SurfaceChat, agents.SurfaceTerminal:
		return true
	default:
		return false
	}
}

// worktreeSpec resolves the create body's worktree mode, writing the error and returning false on
// refusal. Contradictory asks (ownWorktree plus import, or import plus a workspace) are refused rather
// than resolved by precedence, which would hand back a chat on a branch the caller never meant.
func (h *Handlers) worktreeSpec(
	ctx *gin.Context,
	body createRequest,
	wsID string,
) (agentusecase.WorktreeSpec, bool) {
	none := agentusecase.WorktreeSpec{Mode: agentusecase.WorktreeNone}
	if body.Import == nil {
		return forkSpec(ctx, body, wsID)
	}
	if body.OwnWorktree {
		libs.WriteErr(ctx, http.StatusBadRequest,
			"a chat forks a new branch or imports an existing one, not both")
		return none, false
	}
	if wsID != "" {
		libs.WriteErr(ctx, http.StatusBadRequest,
			"a chat attached to a workspace has its worktree already; it cannot also import one")
		return none, false
	}
	spec, ok := h.importSpec(ctx, *body.Import)
	if !ok {
		return none, false
	}
	return agentusecase.WorktreeSpec{Mode: agentusecase.WorktreeImport, Import: spec}, true
}

// forkSpec answers a create with no import: none, or a fresh branch forked from
// the parent row's worktree, which a create with no parent row cannot name.
func forkSpec(
	ctx *gin.Context,
	body createRequest,
	wsID string,
) (agentusecase.WorktreeSpec, bool) {
	none := agentusecase.WorktreeSpec{Mode: agentusecase.WorktreeNone}
	if !body.OwnWorktree || wsID != "" {
		return none, true
	}
	if body.ParentID == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "a new branch forks from its parent row: parentId is required")
		return none, false
	}
	return agentusecase.WorktreeSpec{Mode: agentusecase.WorktreeFork, Branch: body.Branch}, true
}

// importSpec reads the repo facts from :repoId rather than the body, since only the branch and remote
// are the caller's to assert. Git-lineage parents stay empty: a single create has no open-PR graph to
// walk, unlike a batch import.
func (h *Handlers) importSpec(
	ctx *gin.Context,
	in createImportRequest,
) (agentusecase.ImportSpec, bool) {
	if strings.TrimSpace(in.Branch) == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "import.branch is required")
		return agentusecase.ImportSpec{}, false
	}
	if h.repos == nil {
		libs.WriteErr(ctx, http.StatusBadRequest, "importing a branch is not available on this route")
		return agentusecase.ImportSpec{}, false
	}
	repo, err := h.repos.FindByKey(ctx.Request.Context(), ctx.Param("repoId"))
	if err != nil {
		libs.Fail(ctx, err)
		return agentusecase.ImportSpec{}, false
	}
	if repo == nil {
		libs.WriteErr(ctx, http.StatusNotFound, "repo not found")
		return agentusecase.ImportSpec{}, false
	}
	remote := repo.RemoteURL
	if in.Remote != nil {
		remote = *in.Remote
	}
	return agentusecase.ImportSpec{
		RepoID:    repo.ID,
		ProjectID: repo.ProjectID,
		RepoPath:  repo.Path,
		RemoteURL: remote,
		Branch:    in.Branch,
	}, true
}

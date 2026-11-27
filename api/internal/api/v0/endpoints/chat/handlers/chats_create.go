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

type createRequest struct {
	ID          string               `json:"id"`
	Provider    string               `json:"provider"`
	ParentID    string               `json:"parentId"`
	WorkspaceID string               `json:"workspaceId"`
	OwnWorktree bool                 `json:"ownWorktree"`
	Branch      string               `json:"branch"`
	Import      *createImportRequest `json:"import"`
	Surface     string               `json:"surface"`
}

type createImportRequest struct {
	Branch string  `json:"branch"`
	Remote *string `json:"remote"`
}

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

type createdData struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

func knownSurface(surface string) bool {
	switch surface {
	case "", agents.SurfaceChat, agents.SurfaceTerminal:
		return true
	default:
		return false
	}
}

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

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
	// Branch names the fresh branch an ownWorktree create forks — read only
	// when OwnWorktree is true. Blank keeps the server-generated name every
	// caller before this field existed always got; distinct from Import's own
	// Branch, which names a branch that already exists rather than one this
	// create is about to cut.
	Branch string               `json:"branch"`
	Import *createImportRequest `json:"import"`
	// Surface is the view the new chat is born on; omitted means the provider's
	// default. "terminal" births it on the provider's own CLI, whose PTY is the
	// conversation; a provider that cannot start there falls back at spawn.
	Surface string `json:"surface"`
}

// createImportRequest is the import half of the create body. Its PRESENCE is
// what selects WorktreeImport — the three modes are then mutually explicit on
// the wire (nothing, ownWorktree, import) rather than inferred from which
// string happens to be non-empty.
//
// remote is a pointer for the reason lockRequest.Locked is: omitting it and
// sending "" are different answers. Omitted means "the repo's own remote", the
// only sane default for a branch discovered in that repo; an explicit "" means
// a purely local branch with nothing to fetch it from.
type createImportRequest struct {
	Branch string  `json:"branch"`
	Remote *string `json:"remote"`
}

// Create handles POST .../repos/:repoId/chats: spawns a fresh AgentChat and
// starts a RUNNER on it, launching the provider's vendor CLI in a PTY. It
// responds with the new chat's id; the spawned runner id is not surfaced here
// (the client reads it back as liveRunnerId via GET .../chats or .../chats/:id).
//
// The URL no longer names a workspace to anchor the new chat to (Task 17): a
// caller that wants one names it explicitly in the body's workspaceId, which
// defaults to "" — a workspace-less chat, legal since WorkspaceID became
// optional. The home mount still injects :wsId (RequireHomeWorkspace), which
// wins when present so the project home's own create is unaffected.
//
// The optional parentId names where the chat is BORN in the Chats tree: another
// chat (making it a thread of that chat), a folder ("new chat in this folder"), or
// absent/"" for the panel root, which is the original behaviour exactly. It is
// routed through the tree usecase rather than straight to a spawn because the
// placement has to be written BEFORE the CLI starts — a thread is told what it
// reads at spawn, and a thread placed afterwards spends its first session, the one
// the user just asked for, not knowing it is one.
//
// A parentId that names nothing, or a row in another workspace, is refused with the
// same errors a placement returns, and nothing is created.
//
// ownWorktree (model spec §4.1/§5.1) asks for the atomic create: a fresh
// workspace forked from the new chat's resolved fork parent, minted and set on
// the row in the SAME request — see ChatTreeUsecase.CreateChat. It only ever
// applies when the request names no workspace at all: a wsID resolved from
// either the path (:wsId, the home mount) or the body's workspaceId means the
// caller asked to attach to an EXISTING workspace, and that path is unchanged
// regardless of what ownWorktree says.
//
// import is the THIRD way, and this route is where importing a branch becomes a
// chat create rather than a workspace one (spec §4.1: "Create/Import die as
// routes — both are POST /chats with a WorktreeSpec"). It runs the same atomic
// mint→place→attach sequence fork does, differing only in where the worktree
// comes from: an existing branch adopted instead of a fresh one cut.
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

// createdData answers a create: the chat, and the workspace a fork or an
// import minted for it.
type createdData struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

// knownSurface accepts the create body's `surface`: one of the surface names
// design spec 2.5 defines, or "" for the provider's own default landing. It
// is a SPELLING check only — whether the chosen provider can actually launch
// onto that surface is the descriptor's answer, resolved at spawn.
func knownSurface(surface string) bool {
	switch surface {
	case "", agents.SurfaceChat, agents.SurfaceTerminal:
		return true
	default:
		return false
	}
}

// worktreeSpec reads the create body's three mutually exclusive answers to "and
// what worktree does this chat get?", writing the error response and returning
// ok=false when the caller must stop.
//
// A request that asks for TWO is refused rather than resolved by precedence: a
// caller sending both ownWorktree and import has contradicted itself, and
// silently honouring one would hand back a chat on a branch it never asked for
// — cut fresh when it meant to adopt, or the reverse. Same for an import that
// also names a workspace to attach to: naming one means the worktree already
// exists and is not this create's to make.
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

// importSpec describes the branch an importing create adopts, from the body's
// branch plus the repo :repoId names.
//
// The repo facts are read here rather than taken from the caller because they
// are not the caller's to assert: which directory git works in, and which
// project owns it, follow from the repo already in the URL. Only the branch —
// and, optionally, the remote it is fetched from — is something the caller
// knows and the daemon does not.
//
// ParentWorkspaceID and ParentBranch are deliberately left empty. They are the
// GIT LINEAGE parent, which a BATCH import resolves by walking the open-PR
// graph across every branch it was handed at once; a single create has no such
// graph to walk and must not invent one. The chat's own placement still follows
// parentId exactly as every other create's does — the two have always been
// independently written fields (see agentusecase.ImportSpec).
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

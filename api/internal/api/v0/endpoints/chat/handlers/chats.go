package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// createRequest is the POST .../repos/:repoId/chats body. See Create.
type createRequest struct {
	Provider    string               `json:"provider"`
	ParentID    string               `json:"parentId"`
	WorkspaceID string               `json:"workspaceId"`
	OwnWorktree bool                 `json:"ownWorktree"`
	Import      *createImportRequest `json:"import"`
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

	chatID, _, err := h.folders.CreateChat(rctx, wsID, body.Provider, body.ParentID, worktree)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusCreated, chatID)
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
		if body.OwnWorktree && wsID == "" {
			return agentusecase.WorktreeSpec{Mode: agentusecase.WorktreeFork}, true
		}
		return none, true
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
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

// List handles GET .../repos/:repoId/chats, returning every conversation-typed
// chat, each carrying the runner facts derived for it by chatRuntime (its live
// runner, that runner's PTY, its provider) — so the chat list can render
// provider glyphs and attach a pane without a second round trip per row.
//
// A request that still names a workspace (the home mount's injected :wsId)
// scopes to it exactly as before. Otherwise it scopes to the REPO in the URL:
// each row's owning workspace is resolved through the cwd walk and its repo
// compared against :repoId, so a bubble is listed under the repo it actually
// runs in and no chat from another repo is served at all.
func (h *Handlers) List(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	chats, err := h.listChats(rctx, ctx.Param("wsId"), ctx.Param("repoId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	runtimes := make(map[string]dto.ChatRuntime, len(chats))
	for _, c := range chats {
		rt, err := h.chatRuntime(rctx, c.ID)
		if err != nil {
			status, msg := libs.StatusAndMessage(err)
			libs.WriteErr(ctx, status, msg)
			return
		}
		runtimes[c.ID] = rt
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDTOList(
		chats, runtimes, h.repoWorktrees(rctx, ctx.Param("projectId"), ctx.Param("repoId"))))
}

// listChats backs List: wsID scopes to ListChatsByWorkspace when the request
// still names a workspace (the home mount's injected :wsId), otherwise repoID
// scopes to ListChatsInRepo — the walk that resolves each row's owning
// workspace and compares its repo against the one in the URL.
//
// A request naming NEITHER cannot happen on any mounted route (the repo group
// always binds :repoId, the home group always injects :wsId), and is answered
// with nothing rather than with every chat the daemon knows: an unscoped list
// is what this route used to serve by accident, and it is not a scope any
// caller should be able to ask for.
func (h *Handlers) listChats(
	ctx context.Context,
	wsID string,
	repoID string,
) ([]domain.Chat, error) {
	if wsID != "" {
		return h.chats.ListChatsByWorkspace(ctx, wsID)
	}
	if repoID == "" {
		return nil, nil
	}
	return h.chats.ListChatsInRepo(ctx, repoID)
}

// Get handles GET .../repos/:repoId/chats/:id, returning the chat with its
// derived runner facts and the conversations it has hosted — the append-only history
// that succeeds the deleted segment list. 404s (via requireChatInWorkspace) when id
// names an unknown chat, or (for a request that still names a workspace) one
// anchored to a DIFFERENT workspace.
func (h *Handlers) Get(
	ctx *gin.Context,
) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}

	rt, err := h.chatRuntime(ctx.Request.Context(), chat.ID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDetailDTOFrom(
		chat, rt, h.chatWorktree(ctx.Request.Context(), chat)))
}

// chatRuntime derives a chat's process view at read time by joining the two runner
// projections: the runner PLACED on it (if any) and the conversations it has hosted.
// Nothing here is read off the chat aggregate, because a chat stores no process facts.
//
// A dormant chat is NOT an error: agentrunner.ErrNotFound from LiveRunnerForChat means
// no live row exists, which means no PTY exists, which is the liveness answer — so it
// yields a nil LiveRunner and the read continues to the history, whose last entry
// supplies the provider the FE still needs (glyph, dropdown, Resume). Any OTHER error
// is a genuine read failure and propagates: an empty liveRunnerId must mean "dormant"
// and never "the projection broke", or the frontend would silently treat a broken read
// as a dead CLI.
func (h *Handlers) chatRuntime(
	ctx context.Context,
	chatID string,
) (dto.ChatRuntime, error) {
	var live *agents.Runner
	runner, err := h.runners.LiveRunnerForChat(ctx, chatID)
	switch {
	case err == nil:
		live = &runner
	case !errors.Is(err, agentrunner.ErrNotFound):
		return dto.ChatRuntime{}, err
	}

	convs, err := h.runners.ConversationsForChat(ctx, chatID)
	if err != nil {
		return dto.ChatRuntime{}, err
	}

	var attachedSessionID string
	var hasLiveAPIConn bool
	if live != nil {
		attachedSessionID, _ = h.runners.AttachedTerminalSession(live.ID)
		hasLiveAPIConn = h.runners.HasLiveAPIConnection(live.ID)
	}

	// TerminalWait is a plain in-memory read of the detector's standing answer,
	// and it takes no ctx and returns no error for that reason: it never touches a
	// repository, a provider or a PTY on the request path. A daemon whose detector
	// is not running answers the zero verdict, which is the same answer every chat
	// gave before this existed. AttachedTerminalSession and HasLiveAPIConnection are
	// the same kind of read.
	return dto.ChatRuntime{
		LiveRunner:           live,
		Conversations:        convs,
		TerminalWait:         h.runners.TerminalWait(chatID),
		AttachedSessionID:    attachedSessionID,
		HasLiveAPIConnection: hasLiveAPIConn,
	}, nil
}

// requireChatInWorkspace loads chatID, 404ing on an unknown id. When the
// request still names a workspace — the home mount's injected :wsId, since a
// project home's chats stay workspace-scoped — it additionally 404s unless
// chatID belongs to that workspace, exactly as before Task 17. A repo-scoped
// request names no workspace at all, so for it this is an existence check
// only: the model spec addresses a chat by id alone (§5.1), and a chat's
// workspace is optional and mutable, so there is no stale-proof "wrong scope"
// comparison left to make.
//
// It is the by-id scope check shared by Get/Switch/Rename/Handoff (and
// Delete, Task 5): every one of those routes takes a bare chat id with no
// other scoping input. The unknown-id and wrong-workspace cases both return
// HTTP 404 (never the chat body), so no cross-workspace chat is ever served
// to a workspace-scoped caller; the two responses carry DIFFERENT body
// messages ("chat not found in workspace" vs the mapped GetChat not-found
// text), so a probe can still tell "exists elsewhere" from "does not exist" —
// an accepted minor, the scope check's job is to deny cross-workspace ACCESS,
// not to perfectly hide existence. ok is false exactly when the caller must
// return immediately because a response was already written (either this 404
// or a mapped GetChat error).
func (h *Handlers) requireChatInWorkspace(
	ctx *gin.Context,
	chatID string,
) (domain.Chat, bool) {
	chat, err := h.chats.GetChat(ctx.Request.Context(), chatID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return domain.Chat{}, false
	}
	if wsID := ctx.Param("wsId"); wsID != "" && chat.WorkspaceID != wsID {
		libs.WriteErr(ctx, http.StatusNotFound, "chat not found in workspace")
		return domain.Chat{}, false
	}
	return chat, true
}

// Rename handles POST .../repos/:repoId/chats/:id/rename: sets the
// chat's title. `?source=agent` applies the agent precedence rule (skip if
// user-locked); the default (a human/FE rename) sets unconditionally and
// locks. 404s (via requireChatInWorkspace) on an unknown id.
func (h *Handlers) Rename(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")
	source := ctx.Query("source")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.chats.RenameChat(rctx, id, body.Title, source); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteAccepted(ctx)
}

// Promote handles POST .../repos/:repoId/chats/:id/promote: fills a bubble's
// empty workspace slot with a worktree forked from its resolved fork parent and
// respawns its current provider there (model spec §4.2, wire contract §5.1).
//
// It responds with the promoted chat itself rather than an Accepted, because
// the one thing the caller asked about — which workspace the row now owns — is
// only knowable from the answer: the workspace is minted server-side, and the
// row's own walks (ownsWorktree, workspaceId) change with it.
//
// It is a POST on the chat and not a PATCH of its workspaceId for the reason
// the model spec gives promotion its own verb: this is not a field write. It
// cuts a branch, adds a worktree, tears the running CLI down and brings a new
// one up in the new directory — and it is one-way (§4.2, "a worktree is never
// demoted"). 404s (via requireChatInWorkspace) on an unknown id; 409s on a
// chat that already has one.
func (h *Handlers) Promote(
	ctx *gin.Context,
) {
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	promoted, err := h.chats.Promote(ctx.Request.Context(), id)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	rt, err := h.chatRuntime(ctx.Request.Context(), promoted.ID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDetailDTOFrom(
		promoted, rt, h.chatWorktree(ctx.Request.Context(), promoted)))
}

// SetSelection handles PATCH .../repos/:repoId/chats/:id/selection:
// writes the chat's sticky choice of model and reasoning effort.
//
// The body is the WHOLE selection, not a patch of one field: an omitted or empty
// value clears that half back to the provider's own default. The two move
// together because they are not independent — which effort levels are valid is a
// property of the model — so a partial write could store a pair that was never
// jointly valid.
//
// 404s (via requireChatInWorkspace) on an unknown id; 400s when a value is
// outside the provider's declared catalogue.
func (h *Handlers) SetSelection(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	var body struct {
		Model  string `json:"model"`
		Effort string `json:"effort"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.chats.SetChatSelection(rctx, id, body.Model, body.Effort); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteAccepted(ctx)
}

// Delete handles DELETE .../repos/:repoId/chats/:id: hard-deletes the
// chat AND EVERY CHAT THREADED BELOW IT, each through PurgeChat (best-effort PTY
// teardown, then asynx Forget), plus any folder caught inside that subtree. Each
// chat's event log is erased, not merely tombstoned, so it is gone from every
// subsequent read, including a direct GetChat by id.
//
// The cascade is the panel's rule and it is the OPPOSITE of deleting a folder,
// which promotes what it held. A thread is not filed under its parent, it
// CONTINUES it — it reads that chat's turns whenever it asks — so promoting it
// would leave a conversation whose entire premise has been deleted and which no
// drag can restore. That is why this goes through the tree usecase rather than
// straight to PurgeChat: only something holding the tree knows which chats those
// are.
//
// The scoped "deleted" broadcast every client sees for each purged chat comes
// from the hub projection's OnForget (Task 5), not from here; the folders taken
// with them have no projection to ride, so those frames ARE sent from here,
// keyed on :wsId when the request still has one (the home mount) and "" — every
// subscriber, since the WS filter degrades to unfiltered with no :wsId bound —
// otherwise. 404s (via requireChatInWorkspace) on an unknown id.
func (h *Handlers) Delete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")
	wsID := ctx.Param("wsId")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	removed, err := h.folders.DeleteChat(rctx, id)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	for _, folderID := range removed.Folders {
		h.broadcastFolder(folderID, wsID, "folder_deleted")
	}
	h.announceFolders(wsID, removed.Shifted, "folder_updated")
	libs.WriteAccepted(ctx)
}

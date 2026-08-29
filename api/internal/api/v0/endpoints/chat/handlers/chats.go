package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

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
func (h *Handlers) Create(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	var body struct {
		Provider    string `json:"provider"`
		ParentID    string `json:"parentId"`
		WorkspaceID string `json:"workspaceId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	wsID := ctx.Param("wsId")
	if wsID == "" {
		wsID = body.WorkspaceID
	}

	chatID, _, err := h.folders.CreateChat(rctx, wsID, body.Provider, body.ParentID)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusCreated, chatID)
}

// List handles GET .../repos/:repoId/chats, returning every conversation-typed
// chat, each carrying the runner facts derived for it by chatRuntime (its live
// runner, that runner's PTY, its provider) — so the chat list can render
// provider glyphs and attach a pane without a second round trip per row.
//
// A request that still names a workspace (the home mount's injected :wsId)
// scopes to it exactly as before. Otherwise the repo boundary is not yet
// enforced — the same disclosed limitation ChatTreeUsecase.ListInRepo already
// carries for folders (no chat row carries its own repo id yet) — so every
// conversation-typed row the daemon knows is returned.
func (h *Handlers) List(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	chats, err := h.listChats(rctx, ctx.Param("wsId"))
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

	libs.WriteQueryOK(ctx, dto.AgentChatDTOList(chats, runtimes))
}

// listChats backs List: wsID scopes to ListChatsByWorkspace when the request
// still names a workspace (the home mount's injected :wsId), otherwise it
// falls back to the global ListChats, narrowed to conversations.
func (h *Handlers) listChats(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	if wsID != "" {
		return h.chats.ListChatsByWorkspace(ctx, wsID)
	}
	all, err := h.chats.ListChats(ctx)
	if err != nil {
		return nil, err
	}
	return onlyConversations(all), nil
}

// onlyConversations narrows a global chat list to real conversations, dropping
// the folder-typed rows List's own repo-scoped ListFolders already serves.
// Branch/workflow rows pass through unfiltered, matching ListChatsByWorkspace's
// existing behaviour (it forwards its store read raw).
func onlyConversations(
	rows []domain.Chat,
) []domain.Chat {
	out := make([]domain.Chat, 0, len(rows))
	for _, row := range rows {
		if row.Type != domain.ChatTypeFolder {
			out = append(out, row)
		}
	}
	return out
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

	libs.WriteQueryOK(ctx, dto.AgentChatDetailDTOFrom(chat, rt))
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

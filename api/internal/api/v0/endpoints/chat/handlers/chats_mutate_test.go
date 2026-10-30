package handlers_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestDelete_Success proves Delete forwards the path id to the tree usecase's
// cascading delete and responds 202 naming no chat taken beyond the selection.
//
// It goes through the TREE and not straight to PurgeChat because a chat's
// children are threads of it and go with it — see Handlers.Delete.
func TestDelete_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	tree := &fakeChatTree{}
	var frames []folderFrame
	h := newFolderHandlersWith(uc, tree, &frames)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.JSONEq(t, `{"success":true,"data":{"takenChatIds":[]}}`, rec.Body.String())
	assert.Equal(t, "c-1", tree.gotPurge)
	assert.Empty(t, uc.purgeCalls, "the handler must not purge behind the tree's back")
}

// A chat delete takes any folder inside its subtree with it, and those folders
// are a plain row with no projection to announce them — so the handler does.
func TestDelete_AnnouncesTheFoldersTheCascadeTook(t *testing.T) {
	tree := &fakeChatTree{deletion: agentusecase.ChatDeletion{
		Chats:   []string{"c-2", "c-1"},
		Folders: []string{"f-1"},
		Shifted: []domain.Chat{{ID: "f-0", Type: domain.ChatTypeFolder}},
	}}
	var frames []folderFrame
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "c-1", WorkspaceID: "ws-1"}}
	h := newFolderHandlersWith(uc, tree, &frames)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Len(t, frames, 2)
	assert.Equal(t, folderFrame{folderID: "f-1", workspaceID: "ws-1", kind: "folder_deleted"}, frames[0])
	assert.Equal(t, folderFrame{folderID: "f-0", workspaceID: "ws-1", kind: "folder_updated"}, frames[1])
}

// The chats the git cascade took beyond the selection are named in the reply,
// so a client can tell the user what else went.
func TestDelete_NamesTheChatsTheCascadeTookBeyondTheSelection(t *testing.T) {
	tree := &fakeChatTree{deletion: agentusecase.ChatDeletion{Chats: []string{"c-1", "c-9"}, Taken: []string{"c-9"}}}
	var frames []folderFrame
	h := newFolderHandlersWith(&fakeAgentUsecase{}, tree, &frames)
	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.JSONEq(t, `{"success":true,"data":{"takenChatIds":["c-9"]}}`, rec.Body.String())
}

// TestDelete_WrongWorkspace404s proves the by-id scope check
// (requireChatInWorkspace): a chat anchored to a DIFFERENT workspace than the
// :wsId path param 404s before PurgeChat is ever called (Task 5).
func TestDelete_WrongWorkspace404s(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "c-1", WorkspaceID: "ws-other"}}
	tree := &fakeChatTree{}
	var frames []folderFrame
	h := newFolderHandlersWith(uc, tree, &frames)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/projects/p1/repos/r1/workspaces/ws1/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws1"}, {Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.purgeCalls, "PurgeChat must never be called once the scope check 404s")
	assert.Empty(t, tree.gotPurge, "and neither must the cascade behind it")
}

// TestDelete_UsecaseError proves a cascade failure surfaces as a mapped error
// response rather than a 202.
func TestDelete_UsecaseError(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	var frames []folderFrame
	h := newFolderHandlersWith(uc, &fakeChatTree{err: agentchat.ErrNotFound}, &frames)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/missing", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "missing"}}

	h.Delete(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestDelete_RefusesWorkingSubtree proves the tree's unconditional working-row
// refusal reaches the caller as 409, not the generic 500 an unmapped sentinel
// would fall through to.
func TestDelete_RefusesWorkingSubtree(t *testing.T) {
	uc := &fakeAgentUsecase{}
	var frames []folderFrame
	h := newFolderHandlersWith(uc, &fakeChatTree{err: agentusecase.ErrTreeSubtreeWorking}, &frames)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

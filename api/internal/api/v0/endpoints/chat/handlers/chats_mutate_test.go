package handlers_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// TestDelete_Success proves Delete forwards the path id to the tree usecase's
// cascading delete and responds 200 naming no chat taken beyond the selection.
//
// It goes through the TREE and not straight to PurgeChat because a chat's
// children are threads of it and go with it — see Handlers.Delete.
func TestDelete_Success(
	t *testing.T,
) {
	uc := &fakeAgentUsecase{}
	tree := &fakeChatTree{}
	h := newFolderHandlersWith(uc, tree)

	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"success":true,"data":{"takenChatIds":[]}}`, rec.Body.String())
	assert.Equal(t, "c-1", tree.gotPurge)
	assert.Empty(t, uc.purgeCalls, "the handler must not purge behind the tree's back")
}

// The chats the git cascade took beyond the selection are named in the reply,
// so a client can tell the user what else went.
func TestDelete_NamesTheChatsTheCascadeTookBeyondTheSelection(t *testing.T) {
	tree := &fakeChatTree{deletion: agentusecase.ChatDeletion{Chats: []string{"c-1", "c-9"}, Taken: []string{"c-9"}}}
	h := newFolderHandlersWith(&fakeAgentUsecase{}, tree)
	ctx, rec := newTestContext(t, http.MethodDelete, "/v0/chats/c-1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c-1"}}

	h.Delete(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"success":true,"data":{"takenChatIds":["c-9"]}}`, rec.Body.String())
}

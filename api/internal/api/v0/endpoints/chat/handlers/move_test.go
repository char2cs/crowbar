package handlers_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

func TestBranchOut_AcceptsTheRequestAndNamesTheBranch(t *testing.T) {
	tree := &fakeChatTree{}
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/c2/branch-out", []byte(`{"branch":" fix-it "}`))
	ctx.Params = gin.Params{{Key: "id", Value: "c2"}}

	newFolderHandlers(tree).BranchOut(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "c2", tree.gotPlaceID)
	assert.Equal(t, "fix-it", tree.gotBranch)
}

func TestBranchOut_AHomeThreadIsRefusedWithTheReason(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrNoRepoToBranch}
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/c2/branch-out", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c2"}}

	newFolderHandlers(tree).BranchOut(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestAnswerMove_CarriesThePersonsWordToTheUsecase(t *testing.T) {
	tree := &fakeChatTree{}
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/c2/move/answer",
		[]byte(`{"requestId":"m1","allow":false,"reason":" not now "}`))
	ctx.Params = gin.Params{{Key: "id", Value: "c2"}}

	newFolderHandlers(tree).AnswerMove(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "c2", tree.gotPlaceID)
	assert.Equal(t, "m1 false not now", tree.gotAnswer)
}

func TestAnswerMove_AnAnswerNamesItsRequest(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/c2/move/answer", []byte(`{"allow":true}`))
	ctx.Params = gin.Params{{Key: "id", Value: "c2"}}

	newFolderHandlers(&fakeChatTree{}).AnswerMove(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

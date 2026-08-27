package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func permissionLevelRequest(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	ctx, rec := newTestContext(t, http.MethodPut,
		"/v0/projects/p/repos/r/workspaces/ws-1/chats/chat-1/permission-level",
		[]byte(body))
	ctx.Params = gin.Params{
		{Key: "wsId", Value: "ws-1"},
		{Key: "id", Value: "chat-1"},
	}
	return ctx, rec
}

func TestSetChatPermissionLevel_ForwardsTheLevel(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := permissionLevelRequest(t, `{"level":"trusted"}`)

	newChatHandlers(inWorkspace(uc)).SetChatPermissionLevel(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.setChatPermissionLevelCalls, 1)
	assert.Equal(t, "chat-1", uc.setChatPermissionLevelCalls[0].chatID)
	assert.Equal(t, agentusecase.PermissionTrusted, uc.setChatPermissionLevelCalls[0].level)
}

func TestSetChatPermissionLevel_RejectsAMalformedBody(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := permissionLevelRequest(t, `{`)

	newChatHandlers(inWorkspace(uc)).SetChatPermissionLevel(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.setChatPermissionLevelCalls)
}

func TestSetChatPermissionLevel_SurfacesAnUnknownLevel(t *testing.T) {
	uc := &fakeAgentUsecase{setChatPermissionLevelErr: apperr.ErrInvalidArgument}
	ctx, rec := permissionLevelRequest(t, `{"level":"yolo"}`)

	newChatHandlers(inWorkspace(uc)).SetChatPermissionLevel(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSetChatPermissionLevel_WrongWorkspace404s(t *testing.T) {
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-other"}}
	ctx, rec := permissionLevelRequest(t, `{"level":"trusted"}`)

	newChatHandlers(uc).SetChatPermissionLevel(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.setChatPermissionLevelCalls,
		"SetChatPermissionLevel must never be called once the scope check 404s")
}

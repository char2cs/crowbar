package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

func TestGetDefaultPermissionLevel_ReturnsTheCurrentLevel(t *testing.T) {
	uc := &fakeAgentUsecase{defaultLevel: agentusecase.PermissionTrusted}
	ctx, rec := newTestContext(t, http.MethodGet, "/v0/settings/chat/permission-level", nil)

	newChatHandlers(uc).GetDefaultPermissionLevel(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data struct {
			Level string `json:"level"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Equal(t, "trusted", envelope.Data.Level)
}

func TestPutDefaultPermissionLevel_ForwardsTheLevel(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/permission-level",
		[]byte(`{"level":"full-auto"}`))

	newChatHandlers(uc).PutDefaultPermissionLevel(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.setDefaultLevelCalls, 1)
	assert.Equal(t, agentusecase.PermissionFullAuto, uc.setDefaultLevelCalls[0])
}

func TestPutDefaultPermissionLevel_RejectsAMalformedBody(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/permission-level", []byte(`{`))

	newChatHandlers(uc).PutDefaultPermissionLevel(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.setDefaultLevelCalls)
}

func TestPutDefaultPermissionLevel_SurfacesAnUnknownLevel(t *testing.T) {
	uc := &fakeAgentUsecase{setDefaultLevelErr: apperr.ErrInvalidArgument}
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/permission-level",
		[]byte(`{"level":"yolo"}`))

	newChatHandlers(uc).PutDefaultPermissionLevel(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

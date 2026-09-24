package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

func TestGetModelManifestFetchEnabled_ReturnsTheCurrentValue(t *testing.T) {
	uc := &fakeAgentUsecase{manifestFetchEnabled: false}
	ctx, rec := newTestContext(t, http.MethodGet, "/v0/settings/chat/model-manifest-fetch", nil)

	newChatHandlers(uc).GetModelManifestFetchEnabled(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.False(t, envelope.Data.Enabled)
}

func TestPutModelManifestFetchEnabled_ForwardsTheValue(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/model-manifest-fetch",
		[]byte(`{"enabled":false}`))

	newChatHandlers(uc).PutModelManifestFetchEnabled(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.setManifestFetchCalls, 1)
	assert.False(t, uc.setManifestFetchCalls[0])
}

func TestPutModelManifestFetchEnabled_RejectsAMalformedBody(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/model-manifest-fetch", []byte(`{`))

	newChatHandlers(uc).PutModelManifestFetchEnabled(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.setManifestFetchCalls)
}

func TestPutModelManifestFetchEnabled_SurfacesAUsecaseError(t *testing.T) {
	uc := &fakeAgentUsecase{setManifestFetchErr: apperr.ErrInvalidArgument}
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/model-manifest-fetch",
		[]byte(`{"enabled":true}`))

	newChatHandlers(uc).PutModelManifestFetchEnabled(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

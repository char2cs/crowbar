package libs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

func newCtx(
	t *testing.T,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	return ctx, rec
}

func decode(
	t *testing.T,
	body []byte,
) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

func TestWriteQueryOK(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.WriteQueryOK(
		ctx,
		map[string]any{"hello": "world"},
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := decode(t, rec.Body.Bytes())
	assert.Equal(t, true, body["success"])
	assert.NotContains(t, body, "error")
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "world", data["hello"])
}

func TestWriteQueryWithStatus(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.WriteQueryWithStatus(
		ctx,
		http.StatusPartialContent,
		[]string{"a", "b"},
	)

	assert.Equal(t, http.StatusPartialContent, rec.Code)
	body := decode(t, rec.Body.Bytes())
	assert.Equal(t, true, body["success"])
	data, ok := body["data"].([]any)
	require.True(t, ok)
	assert.Len(t, data, 2)
}

func TestWriteQueryOKNilData(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.WriteQueryOK(
		ctx,
		nil,
	)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := decode(t, rec.Body.Bytes())
	assert.Equal(t, true, body["success"])
	assert.NotContains(t, body, "data")
	assert.NotContains(t, body, "error")
}

func TestWriteMutationOK(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.WriteMutationOK(
		ctx,
		http.StatusCreated,
		"11111111-2222-3333-4444-555555555555",
	)

	assert.Equal(t, http.StatusCreated, rec.Code)
	body := decode(t, rec.Body.Bytes())
	assert.Equal(t, true, body["success"])
	assert.NotContains(t, body, "error")
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "11111111-2222-3333-4444-555555555555", data["id"])
}

func TestWriteAccepted(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.WriteAccepted(ctx)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
}

func TestWriteErr(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.WriteErr(
		ctx,
		http.StatusNotFound,
		"workspace not found",
	)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	body := decode(t, rec.Body.Bytes())
	assert.Equal(t, false, body["success"])
	assert.Equal(t, "workspace not found", body["error"])
	assert.NotContains(t, body, "data")
}

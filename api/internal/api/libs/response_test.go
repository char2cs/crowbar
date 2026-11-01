package libs_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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

func TestWriteNoContent(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.WriteNoContent(ctx)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
}

func TestWriteOperation_Answers202WithTheOperation(t *testing.T) {
	ctx, rec := newCtx(t)
	op := domain.Operation{ID: "o1", Kind: domain.OpGitPush, State: domain.OpAccepted, Version: 1}

	libs.WriteOperation(ctx, op)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.True(t, env.Success)
	assert.Equal(t, op, env.Data.Operation)
}

func TestIdempotencyKey_ReadsTheHeader(t *testing.T) {
	ctx, _ := newCtx(t)
	ctx.Request = httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
	assert.Empty(t, libs.IdempotencyKey(ctx))

	ctx.Request.Header.Set("Idempotency-Key", "k1")
	assert.Equal(t, "k1", libs.IdempotencyKey(ctx))
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

func TestWriteErr_AlwaysSetsTheCategoryCode(t *testing.T) {
	ctx, rec := newCtx(t)
	libs.WriteErr(ctx, http.StatusNotFound, "x")
	assert.Equal(t, "not_found", decode(t, rec.Body.Bytes())["code"])

	ctx, rec = newCtx(t)
	libs.WriteErr(ctx, http.StatusTeapot, "x")
	assert.Equal(t, "internal", decode(t, rec.Body.Bytes())["code"])
}

// A delete refused over work at risk keeps the wire shape the client's
// workAtRiskOf reads: code work_at_risk and data.workAtRisk.
func TestFail_WritesTheCodeItsStatusAndData(t *testing.T) {
	ctx, rec := newCtx(t)
	refused := domain.WorkAtRisked([]domain.WorkAtRisk{
		{WorkspaceID: "w1", Branch: "feature/x", UncommittedFiles: 2, UnmergedCommits: 1},
	})

	libs.Fail(ctx, fmt.Errorf("delete cascade: %w", refused))

	assert.Equal(t, http.StatusConflict, rec.Code)
	body := decode(t, rec.Body.Bytes())
	assert.Equal(t, "work_at_risk", body["code"])
	assert.Contains(t, body["error"], "feature/x (2 uncommitted files, 1 unmerged commits)")
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{map[string]any{
		"workspaceId": "w1", "branch": "feature/x", "uncommittedFiles": 2.0, "unmergedCommits": 1.0,
	}}, data["workAtRisk"])
}

func TestFail_AnyOtherErrorCarriesItsCodeAndNoData(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.Fail(ctx, fmt.Errorf("x: %w", problem.ErrNotFound))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	body := decode(t, rec.Body.Bytes())
	assert.Equal(t, "not_found", body["code"])
	assert.Equal(t, "x: not found", body["error"])
	assert.NotContains(t, body, "data")
}

func TestFail_AnUncodedErrorIsInternal(t *testing.T) {
	ctx, rec := newCtx(t)

	libs.Fail(ctx, errors.New("boom"))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	body := decode(t, rec.Body.Bytes())
	assert.Equal(t, "internal", body["code"])
	assert.Equal(t, "boom", body["error"])
}

// The client reads a 5xx's message; the access log keeps the whole chain once,
// and a refusal the client can act on is not logged as a failure.
func TestFail_AServerFailureNotesItsWholeChainForTheLog(t *testing.T) {
	ctx, _ := newCtx(t)
	err := fmt.Errorf("repos: list: %w", errors.New("db down"))

	libs.Fail(ctx, err)

	require.Len(t, ctx.Errors, 1)
	assert.Equal(t, err.Error(), ctx.Errors[0].Error())

	refused, _ := newCtx(t)
	libs.Fail(refused, fmt.Errorf("load: %w", problem.ErrNotFound))
	assert.Empty(t, refused.Errors)
}

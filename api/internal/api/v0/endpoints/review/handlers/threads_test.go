package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func sampleThread() domain.ReviewThread {
	return domain.ReviewThread{
		ID:        "t1",
		WsID:      "ws1",
		FilePath:  "feature.go",
		Side:      domain.ReviewSideRight,
		Status:    domain.ReviewThreadStatusOpen,
		CreatedAt: time.Unix(0, 0).UTC(),
		Messages: []domain.ReviewMessage{
			{
				ID:        "m1",
				Body:      "please rename",
				CreatedAt: time.Unix(0, 0).UTC(),
			},
		},
	}
}

func TestOpenThread_Created(t *testing.T) {
	stub := &stubReview{thread: sampleThread()}
	r := newRouter(stub)

	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads", map[string]any{
		"filePath":   "feature.go",
		"lineNumber": 3,
		"side":       string(domain.ReviewSideRight),
		"body":       "please rename",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "ws1", stub.lastOpen.WsID)
	assert.Equal(t, "feature.go", stub.lastOpen.FilePath)
	assert.Equal(t, 3, stub.lastOpen.LineNumber)

	env := decode(t, w)
	require.True(t, env.Success)
	var got dto.ReviewThreadDTO
	require.NoError(t, jsonUnmarshal(env.Data, &got))
	assert.Equal(t, "t1", got.ID)
	require.Len(t, got.Messages, 1)
}

func TestOpenThread_BadJSON_400(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads", []byte("{"))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOpenThread_MissingFilePath_400(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads", map[string]any{
		"lineNumber": 3,
		"side":       string(domain.ReviewSideRight),
		"body":       "x",
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, decode(t, w).Error, "filePath")
}

func TestOpenThread_MissingSide_400(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads", map[string]any{
		"filePath":   "feature.go",
		"lineNumber": 3,
		"body":       "x",
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, decode(t, w).Error, "side")
}

func TestOpenThread_MissingBody_400(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads", map[string]any{
		"filePath":   "feature.go",
		"lineNumber": 3,
		"side":       string(domain.ReviewSideRight),
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, decode(t, w).Error, "body")
}

func TestOpenThread_UsecaseError_500(t *testing.T) {
	stub := &stubReview{openErr: assertAnError()}
	r := newRouter(stub)
	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads", map[string]any{
		"filePath":   "feature.go",
		"lineNumber": 3,
		"side":       string(domain.ReviewSideRight),
		"body":       "x",
	})
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestReply_OK(t *testing.T) {
	stub := &stubReview{thread: sampleThread()}
	r := newRouter(stub)
	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads/t1/reply",
		map[string]any{"body": "done"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "t1", stub.lastThreadID)
	assert.Equal(t, "done", stub.lastReply)
	assert.True(t, decode(t, w).Success)
}

func TestReply_BadJSON_400(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads/t1/reply", []byte("{"))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReply_MissingBody_400(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads/t1/reply", map[string]any{})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, decode(t, w).Error, "body")
}

func TestReply_UnknownThread_404(t *testing.T) {
	stub := &stubReview{replyErr: apperr.ErrNotFound}
	r := newRouter(stub)
	w := do(t, r, http.MethodPost, "/v0/workspaces/ws1/review/threads/nope/reply",
		map[string]any{"body": "hi"})
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSetThreadResolved_TrueThenFalse(t *testing.T) {
	resolved := sampleThread()
	resolved.Status = domain.ReviewThreadStatusResolved
	stub := &stubReview{thread: resolved}
	r := newRouter(stub)

	w := do(t, r, http.MethodPatch, "/v0/workspaces/ws1/review/threads/t1",
		map[string]any{"isResolved": true})
	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, stub.lastResolved)
	var got dto.ReviewThreadDTO
	require.NoError(t, jsonUnmarshal(decode(t, w).Data, &got))
	assert.True(t, got.IsResolved)

	stub.thread = sampleThread()
	rw := do(t, r, http.MethodPatch, "/v0/workspaces/ws1/review/threads/t1",
		map[string]any{"isResolved": false})
	require.Equal(t, http.StatusOK, rw.Code)
	assert.False(t, stub.lastResolved)
	require.NoError(t, jsonUnmarshal(decode(t, rw).Data, &got))
	assert.False(t, got.IsResolved)
}

func TestSetThreadResolved_BadJSON_400(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPatch, "/v0/workspaces/ws1/review/threads/t1", []byte("{"))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSetThreadResolved_MissingField_400(t *testing.T) {
	r := newRouter(&stubReview{})
	w := do(t, r, http.MethodPatch, "/v0/workspaces/ws1/review/threads/t1", map[string]any{})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, decode(t, w).Error, "isResolved")
}

func TestSetThreadResolved_UnknownThread_404(t *testing.T) {
	stub := &stubReview{resolveErr: apperr.ErrNotFound}
	r := newRouter(stub)
	w := do(t, r, http.MethodPatch, "/v0/workspaces/ws1/review/threads/nope",
		map[string]any{"isResolved": true})
	require.Equal(t, http.StatusNotFound, w.Code)
}

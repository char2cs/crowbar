package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPush_Returns202 asserts the slow-git fail-fast/good-path-async contract: a
// push validates synchronously, returns 202 with an empty body, and runs the git
// push in the background. The post-push state is delivered on the git-status
// WebSocket stream (the existing watcher broadcast), not in the HTTP response.
func TestPush_Returns202(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodPost, ws+"/push", nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
}

// TestSlowGitOps_Return202 asserts every slow git op (push/fetch/pull/merge/
// rebase) returns 202 + async, leaving the outcome to the git-status watcher.
func TestSlowGitOps_Return202(
	t *testing.T,
) {
	r := newRouter()

	assert.Equal(t, http.StatusAccepted, do(r, http.MethodPost, ws+"/push", nil).Code)
	assert.Equal(t, http.StatusAccepted, do(r, http.MethodPost, ws+"/fetch", nil).Code)
	assert.Equal(t, http.StatusAccepted, do(r, http.MethodPost, ws+"/pull", nil).Code)
	assert.Equal(t, http.StatusAccepted, do(r, http.MethodPost, ws+"/merge",
		map[string]any{"branch": "main"}).Code)
	assert.Equal(t, http.StatusAccepted, do(r, http.MethodPost, ws+"/rebase",
		map[string]any{"branch": "main"}).Code)
}

// TestSlowGitOps_ValidationFailsSync_4xx asserts the synchronous fail-fast path:
// a merge/rebase missing its required branch is rejected with a 4xx before any
// 202 or background work.
func TestSlowGitOps_ValidationFailsSync_4xx(
	t *testing.T,
) {
	r := newRouter()

	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/merge", map[string]any{}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/rebase", map[string]any{}).Code)
}

// TestCommit_StillReturns200 asserts the fast write path is unchanged: commit
// validates and runs synchronously, returning 200.
func TestCommit_StillReturns200(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodPost, ws+"/commit", map[string]any{"subject": "feat: add"})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestPush_AsyncErrorBroadcastsLastError asserts a background push failure
// surfaces on the workspace entity via SetLastError, not on the HTTP response
// (the handler already returned 202).
func TestPush_AsyncErrorBroadcastsLastError(
	t *testing.T,
) {
	lastErrors := &fakeLastErrors{called: make(chan struct{}, 1)}
	r := newRouterWithErrors(errGit{}, lastErrors)

	rec := do(r, http.MethodPost, ws+"/push", nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)
	select {
	case <-lastErrors.called:
	case <-time.After(time.Second):
		t.Fatal("expected SetLastError to be called for the failed push")
	}
	assert.Equal(t, "ws1", lastErrors.gotID)
	assert.Equal(t, "boom", lastErrors.gotMsg)
}

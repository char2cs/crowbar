package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestSync_Returns202 asserts the fail-fast/good-path-async contract: a sync of
// a chat holding a worktree validates synchronously, returns 202 with an empty
// body, and runs SyncWorkingTreeState in the background. The synced workspace is
// delivered on the WebSocket stream (blackbox in W13), not in the HTTP response.
func TestSync_Returns202(
	t *testing.T,
) {
	reader := &fakeReader{
		synced:   domain.Workspace{ID: "ws1"},
		syncDone: make(chan struct{}),
	}
	r, _ := newChatRouter(reader, &fakeHierarchy{}, &fakeWorktrees{ws: domain.Workspace{ID: "ws1"}})

	rec := do(r, http.MethodPost, chatBase+"/sync", "")
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	waitClosed(t, reader.syncDone)
	assert.Equal(t, "ws1", reader.gotSync)
}

// TestSyncMissingWorkspace_4xx asserts the synchronous existence check: a sync
// for a chat holding no worktree is rejected on the request path with a 4xx
// before any 202 or background sync.
func TestSyncMissingWorkspace_4xx(
	t *testing.T,
) {
	reader := &fakeReader{}
	r, _ := newChatRouter(reader, &fakeHierarchy{}, &fakeWorktrees{err: apperr.ErrNotFound})

	rec := do(r, http.MethodPost, chatBase+"/sync", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, reader.gotSync, "sync must not run when the workspace is missing")
}

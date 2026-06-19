package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestSync_Returns202 asserts the fail-fast/good-path-async contract: a sync of
// an existing workspace validates synchronously, returns 202 with an empty body,
// and runs SyncWorkingTreeState in the background. The synced workspace is
// delivered on the WebSocket stream (blackbox in W13), not in the HTTP response.
func TestSync_Returns202(
	t *testing.T,
) {
	reader := &fakeReader{
		get:      domain.Workspace{ID: "ws1"},
		synced:   domain.Workspace{ID: "ws1"},
		syncDone: make(chan struct{}),
	}
	r := newRouter(reader, &fakeHierarchy{}, &fakeRepos{repo: &domain.Repository{ID: "r1"}})

	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws1/sync", "")
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
	waitClosed(t, reader.syncDone)
	assert.Equal(t, "ws1", reader.gotSync)
}

// TestSyncMissingWorkspace_4xx asserts the synchronous existence check: a sync
// for an unknown workspace is rejected on the request path with a 4xx before any
// 202 or background sync.
func TestSyncMissingWorkspace_4xx(
	t *testing.T,
) {
	reader := &fakeReader{getErr: apperr.ErrNotFound}
	r := newRouter(reader, &fakeHierarchy{}, &fakeRepos{})

	rec := do(r, http.MethodPost, "/v0/projects/p1/repos/r1/workspaces/ws1/sync", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, reader.gotSync, "sync must not run when the workspace is missing")
}

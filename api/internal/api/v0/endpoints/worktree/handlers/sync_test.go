package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// TestSync_AnswersItsOperation: a sync of a chat holding a worktree validates
// synchronously, then answers 202 with the operation running
// SyncWorkingTreeState; the synced workspace arrives on the WebSocket stream.
func TestSync_AnswersItsOperation(
	t *testing.T,
) {
	reader := &fakeReader{synced: domain.Workspace{ID: "ws1"}}
	r, ops := newChatRouter(reader, &fakeHierarchy{}, &fakeWorktrees{ws: domain.Workspace{ID: "ws1", Branch: "feature/a"}})

	rec := do(r, http.MethodPost, chatBase+"/sync", "")
	op := accepted(t, rec)
	assert.Equal(t, domain.OpWorkspaceSync, op.Kind)
	assert.Equal(t, "Sync feature/a", op.Label)
	assert.Equal(t, domain.OpSucceeded, ops.settled(t, rec).State)
	assert.Equal(t, "ws1", reader.gotSync)
}

// TestSyncMissingWorkspace_4xx asserts the synchronous existence check: a sync
// for a chat holding no worktree is rejected on the request path with a 4xx
// before any 202 or background sync.
func TestSyncMissingWorkspace_4xx(
	t *testing.T,
) {
	reader := &fakeReader{}
	r, _ := newChatRouter(reader, &fakeHierarchy{}, &fakeWorktrees{err: problem.ErrNotFound})

	rec := do(r, http.MethodPost, chatBase+"/sync", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, reader.gotSync, "sync must not run when the workspace is missing")
}

package projections

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// AwaitTombstone is woken by the save that persists the deleted row — the delete
// reactor's ordering gate with no polling. A waiter parked before the delete is
// released by it, and leaves no registration behind.
func TestAwaitTombstone_IsWokenByTheSaveThatPersistsIt(t *testing.T) {
	ctx, ax, st := newRegistered(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", WorktreePath: "/wt/w1/worktree", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspaceProvisioned,
	})
	require.NoError(t, err)
	ax.WaitPublish()

	got := make(chan domain.Workspace, 1)
	go func() {
		tomb, awaitErr := st.AwaitTombstone(ctx, "w1")
		assert.NoError(t, awaitErr)
		got <- tomb
	}()

	_, err = ax.SendWait(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)

	tomb := <-got
	assert.Equal(t, domain.WorkspaceStatusDeleted, tomb.Status)
	assert.Equal(t, "/wt/w1/worktree", tomb.WorktreePath, "the tombstone carries its worktree path")
	st.mu.Lock()
	defer st.mu.Unlock()
	assert.Empty(t, st.tombstoneWaiters, "a released waiter unregisters itself")
}

// A live row does not satisfy the wait; the caller's deadline ends it.
func TestAwaitTombstone_EndsWithTheContext(t *testing.T) {
	ctx, ax, st := newRegistered(t)
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
	})
	require.NoError(t, err)
	ax.WaitPublish()

	bounded, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = st.AwaitTombstone(bounded, "w1")
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// With a hub registered, a persisted tombstone is not enough: the purge waits
// for its frame too. The frame is addressed through the workspace's owning
// chat, which the purge deletes — a purge that won the race left the client a
// ghost row it was never told had gone.
func TestAwaitTombstone_WithAHub_WaitsForTheTombstoneFrame(t *testing.T) {
	ctx, ax, st := newRegistered(t)
	release := make(chan struct{})
	var framed []domain.WorkspaceStatus
	RegisterHub(st,
		func(_ context.Context, ws domain.Workspace) domain.WorkspaceStatus { return ws.Status },
		func(status domain.WorkspaceStatus) {
			if status == domain.WorkspaceStatusDeleted {
				<-release // the frame is still going out
			}
			framed = append(framed, status)
		})
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
	})
	require.NoError(t, err)

	got := make(chan domain.Workspace, 1)
	go func() {
		tomb, awaitErr := st.AwaitTombstone(ctx, "w1")
		assert.NoError(t, awaitErr)
		got <- tomb
	}()
	_, err = ax.Send(ctx, wscmds.Delete{ID: "w1"})
	require.NoError(t, err)

	select {
	case <-got:
		t.Fatal("the tombstone was released for purge before its frame went out")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	assert.Equal(t, domain.WorkspaceStatusDeleted, (<-got).Status)
}

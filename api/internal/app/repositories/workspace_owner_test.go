package repositories_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// revHub keeps each workspace's row as the stream hub does: a row at a lower
// rev than the one it holds is dropped. A held publish waits for release.
type revHub struct {
	mu      sync.Mutex
	rows    map[string]dto.WorkspaceDTO
	revs    map[string]int64
	hold    bool
	held    chan struct{}
	release chan struct{}
}

func newRevHub() *revHub {
	return &revHub{rows: map[string]dto.WorkspaceDTO{}, revs: map[string]int64{}}
}

func (h *revHub) holdNext() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hold, h.held, h.release = true, make(chan struct{}), make(chan struct{})
}

func (h *revHub) publish(w dto.WorkspaceDTO, rev int64) {
	h.mu.Lock()
	if h.hold {
		h.hold = false
		held, release := h.held, h.release
		h.mu.Unlock()
		close(held)
		<-release
		h.mu.Lock()
	}
	defer h.mu.Unlock()
	if rev < h.revs[w.ID] {
		return
	}
	h.rows[w.ID], h.revs[w.ID] = w, rev
}

func (h *revHub) row(id string) dto.WorkspaceDTO {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rows[id]
}

// A republish whose read the owning chat's move overtook is dropped: the row a
// client keeps is drawn where the chat moved to.
func TestRepublishOwnedBy_AnOvertakenReadDoesNotWin(t *testing.T) {
	ctx := context.Background()
	hub := newRevHub()
	c, err := newRepos(t, newAdapter(t), hub.publish)
	require.NoError(t, err)
	_, err = c.Workspace.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b", Provisioning: domain.WorkspacePlaceholder,
	}, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	createAgentChat(t, ctx, c, "c1", "")
	_, err = c.AgentChat.SetWorkspace(ctx, "c1", "w1")
	require.NoError(t, err)
	c.WaitQuiescent()

	hub.holdNext()
	stale := make(chan struct{})
	go func() {
		defer close(stale)
		c.Republish(ctx, "w1")
	}()
	<-hub.held
	require.NoError(t, c.Placement.SetPlacement(ctx, "c1", "f1", 2))
	c.RepublishOwnedBy(ctx, "c1", tree.Placement{ParentID: "f1", Order: 2})
	close(hub.release)
	<-stale

	assert.Equal(t, "f1", hub.row("w1").FolderID)
	assert.Equal(t, 2, hub.row("w1").Order)
}

// A thread made a branch keeps its row: the workspace it now owns is drawn at the
// thread's Tree node, the place a reload reads.
func TestOwnerMoved_ABranchedThreadsWorkspaceIsDrawnWhereTheThreadWas(t *testing.T) {
	ctx := context.Background()
	hub := &captureHub{}
	c, err := newRepos(t, newAdapter(t), hub.publish)
	require.NoError(t, err)
	for _, id := range []string{"w0", "w1"} {
		_, err = c.Workspace.Create(ctx, workspace.CreateInput{
			ID: id, RepoID: "r1", ProjectID: "p1", Branch: id, Provisioning: domain.WorkspacePlaceholder,
		}, time.Unix(1, 0).UTC())
		require.NoError(t, err)
	}
	_, err = c.AgentChat.Create(ctx, agentchat.CreateInput{
		ID: "c1", WorkspaceID: "w0", Type: domain.ChatTypeChat, Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	_, err = c.Placement.Create(ctx, "c1", tree.KindChat, "home", 2)
	require.NoError(t, err)
	c.WaitQuiescent()

	_, err = c.AgentChat.SetWorkspace(ctx, "c1", "w1")
	require.NoError(t, err)
	c.WaitQuiescent()

	row, ok := hub.last("w1")
	require.True(t, ok)
	assert.Equal(t, "c1", row.OwningChatID)
	assert.Equal(t, "home", row.FolderID)
	assert.Equal(t, 2, row.Order)
}
